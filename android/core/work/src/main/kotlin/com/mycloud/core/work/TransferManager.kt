package com.mycloud.core.work

import android.content.Context
import androidx.work.Constraints
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.OutOfQuotaPolicy
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.workDataOf
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import javax.inject.Inject
import javax.inject.Singleton

/** Enqueues transfer work and exposes the live transfer list. The only place
 *  that touches WorkManager directly, so :data/feature layers stay WM-agnostic. */
@Singleton
class TransferManager @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val workManager = WorkManager.getInstance(context)

    private val networkConstraints = Constraints.Builder()
        .setRequiredNetworkType(NetworkType.CONNECTED)
        .build()

    fun enqueueUpload(uri: String, displayName: String, folderId: String?) {
        val request = OneTimeWorkRequestBuilder<UploadWorker>()
            .setInputData(
                workDataOf(
                    TransferKeys.KEY_URI to uri,
                    TransferKeys.KEY_NAME to displayName,
                    TransferKeys.KEY_FOLDER_ID to folderId,
                ),
            )
            .addTag(TransferKeys.TAG_TRANSFER)
            .addTag(TransferKeys.TAG_UPLOAD)
            .addTag(TransferKeys.NAME_TAG_PREFIX + displayName)
            .setConstraints(networkConstraints)
            .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            .build()
        workManager.enqueue(request)
    }

    fun enqueueDownload(fileId: String, displayName: String) {
        val request = OneTimeWorkRequestBuilder<DownloadWorker>()
            .setInputData(
                workDataOf(
                    TransferKeys.KEY_FILE_ID to fileId,
                    TransferKeys.KEY_NAME to displayName,
                ),
            )
            .addTag(TransferKeys.TAG_TRANSFER)
            .addTag(TransferKeys.TAG_DOWNLOAD)
            .addTag(TransferKeys.NAME_TAG_PREFIX + displayName)
            .setConstraints(networkConstraints)
            .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            .build()
        workManager.enqueue(request)
    }

    fun enqueueArchiveDownload(
        fileIds: List<String>,
        folderIds: List<String>,
        displayName: String,
    ) {
        // Ids go to a temp file; Data only carries its path (no size limit).
        val requestFile = ArchiveRequestStore.write(context.filesDir, fileIds, folderIds)
        val request = OneTimeWorkRequestBuilder<ArchiveDownloadWorker>()
            .setInputData(
                workDataOf(
                    TransferKeys.KEY_ARCHIVE_REQUEST to requestFile.absolutePath,
                    TransferKeys.KEY_NAME to displayName,
                ),
            )
            .addTag(TransferKeys.TAG_TRANSFER)
            .addTag(TransferKeys.TAG_DOWNLOAD)
            .addTag(TransferKeys.NAME_TAG_PREFIX + displayName)
            .setConstraints(networkConstraints)
            .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            .build()
        workManager.enqueue(request)
    }

    fun observeTransfers(): Flow<List<WorkInfo>> =
        workManager.getWorkInfosByTagFlow(TransferKeys.TAG_TRANSFER)
}
