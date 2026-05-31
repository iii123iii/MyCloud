package com.mycloud.data.repository

import android.content.Context
import androidx.paging.ExperimentalPagingApi
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.PagingData
import androidx.paging.map
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.database.MyCloudDatabase
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.SortKey
import com.mycloud.core.model.SortOrder
import com.mycloud.core.network.api.FileApi
import com.mycloud.core.network.api.TransferApi
import com.mycloud.core.network.config.ServerUrlSource
import com.mycloud.core.network.dto.UpdateFileRequestDto
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.data.mapper.toDomain
import com.mycloud.data.paging.FileRemoteMediator
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.io.File
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class FileRepositoryImpl @Inject constructor(
    private val fileApi: FileApi,
    private val transferApi: TransferApi,
    private val db: MyCloudDatabase,
    private val json: Json,
    private val serverUrlSource: ServerUrlSource,
    @ApplicationContext private val context: Context,
) : FileRepository {

    @OptIn(ExperimentalPagingApi::class)
    override fun pagedFiles(
        folderId: String?,
        sort: SortKey,
        order: SortOrder,
    ): Flow<PagingData<FileNode>> = Pager(
        config = PagingConfig(pageSize = PAGE_SIZE, enablePlaceholders = false),
        remoteMediator = FileRemoteMediator(folderId, sort, order, PAGE_SIZE, fileApi, db),
        pagingSourceFactory = { db.fileDao().pagingSource(folderId) },
    ).flow.map { pagingData -> pagingData.map { it.toDomain() } }

    override suspend fun deleteFile(id: String): NetworkResult<Unit> {
        val result = safeUnitApiCall(json) { fileApi.deleteFile(id) }
        if (result is NetworkResult.Success) db.fileDao().deleteById(id)
        return result
    }

    override suspend fun setStarred(id: String, starred: Boolean): NetworkResult<Unit> {
        val result = safeApiCall(json) { fileApi.updateFile(id, UpdateFileRequestDto(isStarred = starred)) }
        if (result is NetworkResult.Success) db.fileDao().setStarred(id, starred)
        return result.map { }
    }

    override suspend fun rename(id: String, newName: String): NetworkResult<Unit> {
        val result = safeApiCall(json) { fileApi.updateFile(id, UpdateFileRequestDto(name = newName)) }
        if (result is NetworkResult.Success) db.fileDao().rename(id, newName)
        return result.map { }
    }

    override suspend fun moveFile(id: String, destFolderId: String?): NetworkResult<Unit> {
        // Explicit `null` (not an absent key) is required for "move to root".
        val body = buildJsonObject { put("folder_id", destFolderId) }
        val result = safeUnitApiCall(json) { fileApi.moveFile(id, body) }
        // It left the current folder; drop it locally so it disappears immediately.
        if (result is NetworkResult.Success) db.fileDao().deleteById(id)
        return result
    }

    override suspend fun copyFile(id: String, destFolderId: String?): NetworkResult<Unit> =
        safeUnitApiCall(json) {
            fileApi.copyFile(id, buildJsonObject { put("folder_id", destFolderId) })
        }

    override fun thumbnailUrl(fileId: String, size: Int): String =
        "${serverUrlSource.currentBaseUrl()}api/v2/files/$fileId/thumb?size=$size"

    override fun previewUrl(fileId: String): String =
        "${serverUrlSource.currentBaseUrl()}api/v2/files/$fileId:preview"

    override suspend fun downloadToCache(fileId: String, fileName: String): File? =
        withContext(Dispatchers.IO) {
            runCatching {
                val response = transferApi.download(fileId)
                val body = response.body() ?: return@runCatching null
                val dir = File(context.cacheDir, "previews").apply { mkdirs() }
                // Namespace by id so two files with the same display name don't collide.
                val safeName = fileName.substringAfterLast('/').ifBlank { fileId }
                val out = File(dir, "$fileId-$safeName")
                body.byteStream().use { input -> out.outputStream().use { input.copyTo(it) } }
                out
            }.getOrNull()
        }

    private companion object {
        const val PAGE_SIZE = 50
    }
}
