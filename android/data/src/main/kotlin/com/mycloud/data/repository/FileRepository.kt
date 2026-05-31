package com.mycloud.data.repository

import androidx.paging.PagingData
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.SortKey
import com.mycloud.core.model.SortOrder
import kotlinx.coroutines.flow.Flow
import java.io.File

interface FileRepository {

    /** Room-backed, cursor-paged files for a folder (null = root). */
    fun pagedFiles(folderId: String?, sort: SortKey, order: SortOrder): Flow<PagingData<FileNode>>

    suspend fun deleteFile(id: String): NetworkResult<Unit>

    suspend fun setStarred(id: String, starred: Boolean): NetworkResult<Unit>

    suspend fun rename(id: String, newName: String): NetworkResult<Unit>

    /** Move a file to [destFolderId] (null = the caller's root). */
    suspend fun moveFile(id: String, destFolderId: String?): NetworkResult<Unit>

    /** Copy a file into [destFolderId] (null = the caller's root). */
    suspend fun copyFile(id: String, destFolderId: String?): NetworkResult<Unit>

    /** Authenticated thumbnail/preview URLs (loaded via the shared Coil ImageLoader). */
    fun thumbnailUrl(fileId: String, size: Int = 256): String

    fun previewUrl(fileId: String): String

    /** Downloads a file to a private cache file (e.g. for the PDF viewer). */
    suspend fun downloadToCache(fileId: String, fileName: String): File?
}
