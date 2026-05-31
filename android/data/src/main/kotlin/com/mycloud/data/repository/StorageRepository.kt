package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.StorageStats
import com.mycloud.core.network.api.StorageApi
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface StorageRepository {
    suspend fun stats(): NetworkResult<StorageStats>
}

@Singleton
class StorageRepositoryImpl @Inject constructor(
    private val storageApi: StorageApi,
    private val json: Json,
) : StorageRepository {

    override suspend fun stats(): NetworkResult<StorageStats> =
        safeApiCall(json) { storageApi.stats() }.map { it.toDomain() }
}
