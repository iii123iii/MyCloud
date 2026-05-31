package com.mycloud.core.network.api

import com.mycloud.core.network.dto.AuthTokensDto
import com.mycloud.core.network.dto.RefreshRequestDto
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.POST

/**
 * Refresh endpoint, deliberately built on the bare ([Unauthenticated]) client so
 * the [com.mycloud.core.network.interceptor.TokenAuthenticator] can call it
 * without re-entering the authenticator on its own 401.
 */
interface TokenRefreshApi {

    @POST("api/v2/auth/refresh")
    suspend fun refresh(@Body body: RefreshRequestDto): Response<Envelope<AuthTokensDto>>
}
