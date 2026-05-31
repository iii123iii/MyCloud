package com.mycloud.core.network.api

import com.mycloud.core.network.dto.ActivityResponse
import com.mycloud.core.network.dto.CreateTokenRequestDto
import com.mycloud.core.network.dto.SessionsResponse
import com.mycloud.core.network.dto.TokenDto
import com.mycloud.core.network.dto.TokensResponse
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/** Account/self endpoints: sessions, activity log, and personal access tokens. */
interface AccountApi {

    @GET("api/v2/me/sessions")
    suspend fun listSessions(): Response<Envelope<SessionsResponse>>

    @DELETE("api/v2/me/sessions/{jti}")
    suspend fun revokeSession(@Path("jti") jti: String): Response<Unit>

    @GET("api/v2/auth/me/activity")
    suspend fun activity(@Query("limit") limit: Int = 50): Response<Envelope<ActivityResponse>>

    @GET("api/v2/me/tokens")
    suspend fun listTokens(): Response<Envelope<TokensResponse>>

    @POST("api/v2/me/tokens")
    suspend fun createToken(@Body body: CreateTokenRequestDto): Response<Envelope<TokenDto>>

    @DELETE("api/v2/me/tokens/{id}")
    suspend fun revokeToken(@Path("id") id: String): Response<Unit>
}
