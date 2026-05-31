package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.Photo
import com.mycloud.core.network.api.PhotoApi
import com.mycloud.core.network.config.ServerUrlSource
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface PhotoRepository {
    suspend fun photos(): NetworkResult<List<Photo>>
    fun thumbnailUrl(id: String, size: Int = 256): String
    fun previewUrl(id: String): String
}

@Singleton
class PhotoRepositoryImpl @Inject constructor(
    private val photoApi: PhotoApi,
    private val json: Json,
    private val serverUrlSource: ServerUrlSource,
) : PhotoRepository {

    override suspend fun photos(): NetworkResult<List<Photo>> =
        safeApiCall(json) { photoApi.listPhotos() }.map { it.photos.map { dto -> dto.toDomain() } }

    override fun thumbnailUrl(id: String, size: Int): String =
        "${serverUrlSource.currentBaseUrl()}api/v2/files/$id/thumb?size=$size"

    override fun previewUrl(id: String): String =
        "${serverUrlSource.currentBaseUrl()}api/v2/files/$id:preview"
}
