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
import kotlinx.coroutines.CancellationException
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
        // Track the destination so a cancelled/failed download doesn't leave a
        // half-written file behind.
        var outFile: File? = null

        return try {
            setForeground(notifications.foregroundInfo(id, name, 0, upload = false))

            val response = transferApi.download(fileId)
            val responseBody = response.body()
            if (!response.isSuccessful || responseBody == null) {
                return if (response.code() in 500..599 && runAttemptCount < MAX_ATTEMPTS) {
                    Result.retry()
                } else {
                    notifications.notifyComplete(notificationId, name, success = false, upload = false)
                    Result.failure()
                }
            }

            val total = responseBody.contentLength()
            val dir = applicationContext.downloadOutputDir()
            val file = dir.reserveUniqueFile(name.safeOutputFileName("download"))
            outFile = file
            var downloaded = 0L
            var lastPct = -1
            var lastNotifyMs = 0L
            responseBody.byteStream().use { input ->
                file.outputStream().use { output ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    var read = input.read(buffer)
                    while (read != -1) {
                        output.write(buffer, 0, read)
                        downloaded += read
                        val pct = if (total > 0) ((downloaded * 100) / total).toInt() else 0
                        if (pct != lastPct) {
                            lastPct = pct
                            setProgressAsync(workDataOf(TransferKeys.KEY_PROGRESS to pct))
                            lastNotifyMs = notifications.notifyProgressThrottled(
                                notificationId, name, pct, upload = false, lastEmitMs = lastNotifyMs,
                            )
                        }
                        read = input.read(buffer)
                    }
                }
            }

            val outUri = FileProvider.getUriForFile(
                applicationContext,
                "${applicationContext.packageName}.fileprovider",
                file,
            )
            notifications.notifyComplete(notificationId, name, success = true, upload = false)
            Result.success(workDataOf(TransferKeys.KEY_OUTPUT_URI to outUri.toString()))
        } catch (e: CancellationException) {
            // Stopped (e.g. logout cancels TAG_TRANSFER work): clean up the partial.
            runCatching { outFile?.delete() }
            throw e
        } catch (e: Exception) {
            runCatching { outFile?.delete() }
            if (runAttemptCount < MAX_ATTEMPTS) {
                Result.retry()
            } else {
                notifications.notifyComplete(notificationId, name, success = false, upload = false)
                Result.failure()
            }
        }
    }

    private fun displayName(): String = inputData.getString(TransferKeys.KEY_NAME) ?: "download"

    private fun Context.downloadOutputDir(): File =
        getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
            ?: File(cacheDir, "downloads").apply { mkdirs() }

    private fun String.safeOutputFileName(fallback: String): String =
        replace('/', '_').replace('\\', '_').ifBlank { fallback }

    private fun File.reserveUniqueFile(fileName: String): File {
        mkdirs()
        val dot = fileName.lastIndexOf('.').takeIf { it > 0 }
        val base = dot?.let { fileName.substring(0, it) } ?: fileName
        val ext = dot?.let { fileName.substring(it) } ?: ""
        var index = 0
        while (true) {
            val candidateName = if (index == 0) fileName else "$base ($index)$ext"
            val candidate = File(this, candidateName)
            if (candidate.createNewFile()) return candidate
            index++
        }
    }

    private companion object {
        const val MAX_ATTEMPTS = 3
        const val DEFAULT_BUFFER_SIZE = 8 * 1024
    }
}
