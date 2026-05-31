package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.model.Breadcrumb
import com.mycloud.core.model.Folder
import kotlinx.coroutines.flow.Flow

interface FolderRepository {

    /** Room-backed children of a folder (null = root). */
    fun folders(parentId: String?): Flow<List<Folder>>

    /** Fetch children from the server and refresh the cache. */
    suspend fun refreshFolders(parentId: String?): NetworkResult<Unit>

    suspend fun breadcrumb(folderId: String): NetworkResult<List<Breadcrumb>>

    suspend fun createFolder(name: String, parentId: String?): NetworkResult<Unit>

    suspend fun renameFolder(id: String, newName: String): NetworkResult<Unit>

    /** Move a folder under [destParentId] (null = root). */
    suspend fun moveFolder(id: String, destParentId: String?): NetworkResult<Unit>

    /** Copy a folder subtree under [destParentId] (null = root). */
    suspend fun copyFolder(id: String, destParentId: String?): NetworkResult<Unit>

    suspend fun deleteFolder(id: String): NetworkResult<Unit>
}
