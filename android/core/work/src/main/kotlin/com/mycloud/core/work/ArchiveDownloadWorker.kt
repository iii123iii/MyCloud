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
import com.mycloud.core.network.dto.DownloadArchiveRequestDto
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.io.File

/**
 * Streams a server-built zip of selected files + folder subtrees
 * (`POST /files:download-archive`) into the app's external Downloads dir. The zip
 * is chunked, so there's usually no Content-Length — progress is indeterminate.
 */
@HiltWorker
class ArchiveDownloadWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val transferApi: TransferApi,
    private val notifications: TransferNotifications,
) : CoroutineWorker(appContext, params) {

    override suspend fun getForegroundInfo(): ForegroundInfo =
        notifications.foregroundInfo(id, displayName(), progress = 0, upload = false)

    override suspend fun doWork(): Result {
        val requestPath = inputData.getString(TransferKeys.KEY_ARCHIVE_REQUEST)
        val requestFile = requestPath?.let { File(it) }
        val selection = requestFile?.let { ArchiveRequestStore.read(it) }
        if (selection == null) {
            requestFile?.delete()
            return Result.failure()
        }
        val (fileIds, folderIds) = selection
        val name = displayName()
        val notificationId = id.hashCode()

        return try {
            setForeground(notifications.foregroundInfo(id, name, 0, upload = false))

            val response = transferApi.downloadArchive(
                DownloadArchiveRequestDto(fileIds = fileIds, folderIds = folderIds),
            )
            val responseBody = response.body()
            if (!response.isSuccessful || responseBody == null) {
                // Keep the request file for a retry; only clean up on a terminal failure.
                val retryable = response.code() in 500..599 && runAttemptCount < MAX_ATTEMPTS
                if (!retryable) {
                    requestFile.delete()
                    notifications.notifyComplete(notificationId, name, success = false, upload = false)
                }
                return if (retryable) Result.retry() else Result.failure()
            }

            val dir = applicationContext.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
            val outFile = File(dir, name)
            responseBody.byteStream().use { input ->
                outFile.outputStream().use { output -> input.copyTo(output) }
            }

            val outUri = FileProvider.getUriForFile(
                applicationContext,
                "${applicationContext.packageName}.fileprovider",
                outFile,
            )
            requestFile.delete()
            notifications.notifyComplete(notificationId, name, success = true, upload = false)
            Result.success(workDataOf(TransferKeys.KEY_OUTPUT_URI to outUri.toString()))
        } catch (e: Exception) {
            if (runAttemptCount < MAX_ATTEMPTS) {
                Result.retry() // keep the request file so the retry can re-read it
            } else {
                requestFile.delete()
                notifications.notifyComplete(notificationId, name, success = false, upload = false)
                Result.failure()
            }
        }
    }

    private fun displayName(): String = inputData.getString(TransferKeys.KEY_NAME) ?: "mycloud.zip"

    private companion object {
        const val MAX_ATTEMPTS = 3
    }
}
