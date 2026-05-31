package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.ActivityEntry
import com.mycloud.core.model.PersonalAccessToken
import com.mycloud.core.model.Session
import com.mycloud.core.network.api.AccountApi
import com.mycloud.core.network.dto.CreateTokenRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface AccountRepository {
    suspend fun sessions(): NetworkResult<List<Session>>
    suspend fun revokeSession(jti: String): NetworkResult<Unit>
    suspend fun activity(): NetworkResult<List<ActivityEntry>>
    suspend fun tokens(): NetworkResult<List<PersonalAccessToken>>
    /** Returns the one-time plaintext token on success. */
    suspend fun createToken(name: String, scopes: List<String>): NetworkResult<String?>
    suspend fun revokeToken(id: String): NetworkResult<Unit>
}

@Singleton
class AccountRepositoryImpl @Inject constructor(
    private val accountApi: AccountApi,
    private val json: Json,
) : AccountRepository {

    override suspend fun sessions(): NetworkResult<List<Session>> =
        safeApiCall(json) { accountApi.listSessions() }.map { it.sessions.map { dto -> dto.toDomain() } }

    override suspend fun revokeSession(jti: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { accountApi.revokeSession(jti) }

    override suspend fun activity(): NetworkResult<List<ActivityEntry>> =
        safeApiCall(json) { accountApi.activity() }.map { it.logs.map { dto -> dto.toDomain() } }

    override suspend fun tokens(): NetworkResult<List<PersonalAccessToken>> =
        safeApiCall(json) { accountApi.listTokens() }.map { it.tokens.map { dto -> dto.toDomain() } }

    override suspend fun createToken(name: String, scopes: List<String>): NetworkResult<String?> =
        safeApiCall(json) { accountApi.createToken(CreateTokenRequestDto(name, scopes)) }.map { it.token }

    override suspend fun revokeToken(id: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { accountApi.revokeToken(id) }
}
