package com.mycloud.data.repository

import androidx.room.withTransaction
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.database.MyCloudDatabase
import com.mycloud.core.model.Breadcrumb
import com.mycloud.core.model.Folder
import com.mycloud.core.network.api.FolderApi
import com.mycloud.core.network.dto.CreateFolderRequestDto
import com.mycloud.core.network.dto.UpdateFolderRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import com.mycloud.data.mapper.toEntity
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class FolderRepositoryImpl @Inject constructor(
    private val folderApi: FolderApi,
    private val db: MyCloudDatabase,
    private val json: Json,
) : FolderRepository {

    override fun folders(parentId: String?): Flow<List<Folder>> =
        db.folderDao().observeChildren(parentId).map { list -> list.map { it.toDomain() } }

    override suspend fun refreshFolders(parentId: String?): NetworkResult<Unit> {
        val result = safeApiCall(json) { folderApi.listFolders(parentId) }
        if (result is NetworkResult.Success) {
            db.withTransaction {
                db.folderDao().clearChildren(parentId)
                db.folderDao().upsertAll(result.data.folders.map { it.toEntity() })
            }
        }
        return result.map { }
    }

    override suspend fun breadcrumb(folderId: String): NetworkResult<List<Breadcrumb>> =
        safeApiCall(json) { folderApi.folderPath(folderId) }
            .map { response -> response.path.map { it.toDomain() } }

    override suspend fun createFolder(name: String, parentId: String?): NetworkResult<Unit> {
        val result = safeApiCall(json) { folderApi.createFolder(CreateFolderRequestDto(name, parentId)) }
        if (result is NetworkResult.Success) refreshFolders(parentId)
        return result.map { }
    }

    override suspend fun renameFolder(id: String, newName: String): NetworkResult<Unit> =
        safeApiCall(json) { folderApi.updateFolder(id, UpdateFolderRequestDto(name = newName)) }.map { }

    override suspend fun moveFolder(id: String, destParentId: String?): NetworkResult<Unit> {
        val body = buildJsonObject { put("parent_id", destParentId) }
        val result = safeUnitApiCall(json) { folderApi.moveFolder(id, body) }
        if (result is NetworkResult.Success) db.folderDao().deleteById(id)
        return result
    }

    override suspend fun copyFolder(id: String, destParentId: String?): NetworkResult<Unit> =
        safeUnitApiCall(json) {
            folderApi.copyFolder(id, buildJsonObject { put("parent_id", destParentId) })
        }

    override suspend fun deleteFolder(id: String): NetworkResult<Unit> {
        val result = safeUnitApiCall(json) { folderApi.deleteFolder(id) }
        if (result is NetworkResult.Success) db.folderDao().deleteById(id)
        return result
    }
}
