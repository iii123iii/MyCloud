package com.mycloud.data.repository

import com.mycloud.core.model.Transfer
import com.mycloud.core.work.TransferManager
import com.mycloud.data.mapper.toTransfer
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

interface TransferRepository {
    fun transfers(): Flow<List<Transfer>>
    fun uploadFile(uri: String, displayName: String, folderId: String?)
    fun download(fileId: String, displayName: String)

    /** Download a server-built zip of the selected files + folder subtrees. */
    fun downloadArchive(fileIds: List<String>, folderIds: List<String>, displayName: String)
}

@Singleton
class TransferRepositoryImpl @Inject constructor(
    private val transferManager: TransferManager,
) : TransferRepository {

    override fun transfers(): Flow<List<Transfer>> =
        transferManager.observeTransfers().map { infos -> infos.map { it.toTransfer() } }

    override fun uploadFile(uri: String, displayName: String, folderId: String?) =
        transferManager.enqueueUpload(uri, displayName, folderId)

    override fun download(fileId: String, displayName: String) =
        transferManager.enqueueDownload(fileId, displayName)

    override fun downloadArchive(fileIds: List<String>, folderIds: List<String>, displayName: String) =
        transferManager.enqueueArchiveDownload(fileIds, folderIds, displayName)
}
