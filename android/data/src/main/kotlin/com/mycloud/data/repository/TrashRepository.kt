package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.TrashItem
import com.mycloud.core.network.api.TrashApi
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.isoToEpochMillis
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface TrashRepository {
    suspend fun list(): NetworkResult<List<TrashItem>>
    suspend fun restore(id: String): NetworkResult<Unit>
    suspend fun deleteForever(id: String): NetworkResult<Unit>
    suspend fun empty(): NetworkResult<Unit>
}

@Singleton
class TrashRepositoryImpl @Inject constructor(
    private val trashApi: TrashApi,
    private val json: Json,
) : TrashRepository {

    override suspend fun list(): NetworkResult<List<TrashItem>> =
        safeApiCall(json) { trashApi.listTrash() }.map { response ->
            response.items.map {
                TrashItem(
                    type = it.type,
                    id = it.id,
                    name = it.name,
                    sizeBytes = it.sizeBytes,
                    mimeType = it.mimeType,
                    deletedAtMillis = it.deletedAt.isoToEpochMillis(),
                )
            }
        }

    override suspend fun restore(id: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { trashApi.restore(id) }

    override suspend fun deleteForever(id: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { trashApi.deleteForever(id) }

    override suspend fun empty(): NetworkResult<Unit> =
        safeUnitApiCall(json) { trashApi.empty() }
}
