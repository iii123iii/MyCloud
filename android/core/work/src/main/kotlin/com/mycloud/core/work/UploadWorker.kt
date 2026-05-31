package com.mycloud.core.work

import android.content.ContentResolver
import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import com.mycloud.core.network.api.TransferApi
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody

/** Multipart upload of a picked content URI, with progress + a foreground notification. */
@HiltWorker
class UploadWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val transferApi: TransferApi,
    private val notifications: TransferNotifications,
) : CoroutineWorker(appContext, params) {

    override suspend fun getForegroundInfo(): ForegroundInfo =
        notifications.foregroundInfo(id, displayName(), progress = 0, upload = true)

    override suspend fun doWork(): Result {
        val uriString = inputData.getString(TransferKeys.KEY_URI) ?: return Result.failure()
        val name = displayName()
        val folderId = inputData.getString(TransferKeys.KEY_FOLDER_ID)
        val uri = Uri.parse(uriString)
        val resolver = applicationContext.contentResolver
        val mime = resolver.getType(uri) ?: "application/octet-stream"
        val size = resolver.fileSize(uri)
        val notificationId = id.hashCode()

        return try {
            setForeground(notifications.foregroundInfo(id, name, 0, upload = true))

            var lastPct = -1
            val body = UriRequestBody(resolver, uri, mime, size) { sent, total ->
                val pct = if (total > 0) ((sent * 100) / total).toInt() else 0
                if (pct != lastPct) {
                    lastPct = pct
                    setProgressAsync(workDataOf(TransferKeys.KEY_PROGRESS to pct))
                    notifications.notifyProgress(notificationId, name, pct, upload = true)
                }
            }
            val filePart = MultipartBody.Part.createFormData("file", name, body)
            val folderPart = folderId?.toRequestBody("text/plain".toMediaType())

            // folder_id first — see TransferApi.upload for why ordering matters.
            val response = transferApi.upload(folderPart, filePart)
            if (response.isSuccessful) {
                notifications.notifyComplete(notificationId, name, success = true, upload = true)
                Result.success(workDataOf(TransferKeys.KEY_FILE_ID to response.body()?.data?.id))
            } else {
                notifications.notifyComplete(notificationId, name, success = false, upload = true)
                if (response.code() in 500..599 && runAttemptCount < MAX_ATTEMPTS) {
                    Result.retry()
                } else {
                    Result.failure()
                }
            }
        } catch (e: Exception) {
            if (runAttemptCount < MAX_ATTEMPTS) {
                Result.retry()
            } else {
                notifications.notifyComplete(notificationId, name, success = false, upload = true)
                Result.failure()
            }
        }
    }

    private fun displayName(): String = inputData.getString(TransferKeys.KEY_NAME) ?: "file"

    private fun ContentResolver.fileSize(uri: Uri): Long {
        return runCatching {
            query(uri, arrayOf(OpenableColumns.SIZE), null, null, null)?.use { cursor ->
                val index = cursor.getColumnIndex(OpenableColumns.SIZE)
                if (index >= 0 && cursor.moveToFirst()) cursor.getLong(index) else -1L
            } ?: -1L
        }.getOrDefault(-1L)
    }

    private companion object {
        const val MAX_ATTEMPTS = 3
    }
}
