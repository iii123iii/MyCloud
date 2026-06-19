package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.Grant
import com.mycloud.core.model.Permission
import com.mycloud.core.model.Share
import com.mycloud.core.network.api.ShareApi
import com.mycloud.core.network.config.ServerUrlSource
import com.mycloud.core.network.dto.CreateGrantRequestDto
import com.mycloud.core.network.dto.CreateShareRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface ShareRepository {
    suspend fun listShares(): NetworkResult<List<Share>>
    suspend fun createShare(
        fileId: String?,
        folderId: String?,
        permission: Permission,
        password: String?,
        expiresAtIso: String?,
        downloadLimit: Long?,
        singleView: Boolean,
    ): NetworkResult<Share>
    suspend fun deleteShare(id: String): NetworkResult<Unit>

    /**
     * List per-user grants. [direction] = "outgoing" returns grants the current
     * user created (shared by me); "incoming" returns grants shared WITH the user.
     */
    suspend fun listGrants(direction: String = "outgoing"): NetworkResult<List<Grant>>

    /** Grant a person (by username or email) access to a file or folder. */
    suspend fun createGrant(
        fileId: String?,
        folderId: String?,
        grantee: String,
        permission: Permission,
    ): NetworkResult<Unit>

    suspend fun deleteGrant(id: String): NetworkResult<Unit>

    /** A copyable public link for a share token. */
    fun publicLink(token: String): String
}

@Singleton
class ShareRepositoryImpl @Inject constructor(
    private val shareApi: ShareApi,
    private val json: Json,
    private val serverUrlSource: ServerUrlSource,
) : ShareRepository {

    override suspend fun listShares(): NetworkResult<List<Share>> =
        safeApiCall(json) { shareApi.listShares() }.map { it.shares.map { dto -> dto.toDomain() } }

    override suspend fun createShare(
        fileId: String?,
        folderId: String?,
        permission: Permission,
        password: String?,
        expiresAtIso: String?,
        downloadLimit: Long?,
        singleView: Boolean,
    ): NetworkResult<Share> = safeApiCall(json) {
        shareApi.createShare(
            CreateShareRequestDto(
                fileId = fileId,
                folderId = folderId,
                permission = permission.name.lowercase(),
                password = password?.ifBlank { null },
                expiresAt = expiresAtIso,
                downloadLimit = downloadLimit,
                singleView = singleView,
            ),
        )
    }.map { it.toDomain() }

    override suspend fun deleteShare(id: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { shareApi.deleteShare(id) }

    override suspend fun listGrants(direction: String): NetworkResult<List<Grant>> =
        safeApiCall(json) { shareApi.listGrants(direction = direction) }
            .map { it.grants.map { dto -> dto.toDomain() } }

    override suspend fun createGrant(
        fileId: String?,
        folderId: String?,
        grantee: String,
        permission: Permission,
    ): NetworkResult<Unit> = safeApiCall(json) {
        shareApi.createGrant(
            CreateGrantRequestDto(
                grantee = grantee.trim(),
                fileId = fileId,
                folderId = folderId,
                permission = permission.toGrantWire(),
            ),
        )
    }.map { }

    override suspend fun deleteGrant(id: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { shareApi.deleteGrant(id) }

    override fun publicLink(token: String): String = "${serverUrlSource.currentBaseUrl()}s/$token"
}

/** Map the shared [Permission] enum onto the grant vocabulary (viewer/editor/owner). */
private fun Permission.toGrantWire(): String = when (this) {
    Permission.WRITE -> "editor"
    Permission.OWNER -> "owner"
    Permission.READ -> "viewer"
}
