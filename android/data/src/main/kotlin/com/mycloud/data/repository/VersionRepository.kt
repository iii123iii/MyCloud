package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.FileVersion
import com.mycloud.core.network.api.VersionApi
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface VersionRepository {
    suspend fun list(fileId: String): NetworkResult<List<FileVersion>>
    suspend fun restore(fileId: String, versionNumber: Int): NetworkResult<Unit>
}

@Singleton
class VersionRepositoryImpl @Inject constructor(
    private val versionApi: VersionApi,
    private val json: Json,
) : VersionRepository {

    override suspend fun list(fileId: String): NetworkResult<List<FileVersion>> =
        safeApiCall(json) { versionApi.listVersions(fileId) }
            .map { it.versions.map { dto -> dto.toDomain() } }

    override suspend fun restore(fileId: String, versionNumber: Int): NetworkResult<Unit> =
        safeUnitApiCall(json) { versionApi.restoreVersion(fileId, versionNumber) }
}
