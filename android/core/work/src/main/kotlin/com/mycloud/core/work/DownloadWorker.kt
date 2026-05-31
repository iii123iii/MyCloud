package com.mycloud.core.work

import android.content.Context
import android.os.Environment
import androidx.core.content.FileProvider
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import com.mycloud.core.network.api.TransferApi
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.io.File

/** Streams a file download into the app's external Downloads dir and exposes it
 *  via FileProvider for open/share. (SAF/MediaStore destinations are a polish item.) */
@HiltWorker
class DownloadWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val transferApi: TransferApi,
    private val notifications: TransferNotifications,
) : CoroutineWorker(appContext, params) {

    override suspend fun getForegroundInfo(): ForegroundInfo =
        notifications.foregroundInfo(id, displayName(), progress = 0, upload = false)

    override suspend fun doWork(): Result {
        val fileId = inputData.getString(TransferKeys.KEY_FILE_ID) ?: return Result.failure()
        val name = displayName()
        val notificationId = id.hashCode()

        return try {
            setForeground(notifications.foregroundInfo(id, name, 0, upload = false))

            val response = transferApi.download(fileId)
            val responseBody = response.body()
            if (!response.isSuccessful || responseBody == null) {
                notifications.notifyComplete(notificationId, name, success = false, upload = false)
                return if (response.code() in 500..599 && runAttemptCount < MAX_ATTEMPTS) {
                    Result.retry()
                } else {
                    Result.failure()
                }
            }

            val total = responseBody.contentLength()
            val dir = applicationContext.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
            val outFile = File(dir, name)
            var downloaded = 0L
            var lastPct = -1
            responseBody.byteStream().use { input ->
                outFile.outputStream().use { output ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    var read = input.read(buffer)
                    while (read != -1) {
                        output.write(buffer, 0, read)
                        downloaded += read
                        val pct = if (total > 0) ((downloaded * 100) / total).toInt() else 0
                        if (pct != lastPct) {
                            lastPct = pct
                            setProgressAsync(workDataOf(TransferKeys.KEY_PROGRESS to pct))
                            notifications.notifyProgress(notificationId, name, pct, upload = false)
                        }
                        read = input.read(buffer)
                    }
                }
            }

            val outUri = FileProvider.getUriForFile(
                applicationContext,
                "${applicationContext.packageName}.fileprovider",
                outFile,
            )
            notifications.notifyComplete(notificationId, name, success = true, upload = false)
            Result.success(workDataOf(TransferKeys.KEY_OUTPUT_URI to outUri.toString()))
        } catch (e: Exception) {
            if (runAttemptCount < MAX_ATTEMPTS) {
                Result.retry()
            } else {
                notifications.notifyComplete(notificationId, name, success = false, upload = false)
                Result.failure()
            }
        }
    }

    private fun displayName(): String = inputData.getString(TransferKeys.KEY_NAME) ?: "download"

    private companion object {
        const val MAX_ATTEMPTS = 3
        const val DEFAULT_BUFFER_SIZE = 8 * 1024
    }
}
