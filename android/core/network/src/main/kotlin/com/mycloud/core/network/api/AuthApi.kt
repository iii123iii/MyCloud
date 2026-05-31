package com.mycloud.core.network.api

import com.mycloud.core.network.dto.AuthTokensDto
import com.mycloud.core.network.dto.ChangePasswordRequestDto
import com.mycloud.core.network.dto.DeleteAccountRequestDto
import com.mycloud.core.network.dto.LoginRequestDto
import com.mycloud.core.network.dto.LogoutRequestDto
import com.mycloud.core.network.dto.MessageDto
import com.mycloud.core.network.dto.UserDto
import com.mycloud.core.network.dto.WhoamiDto
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.HTTP
import retrofit2.http.POST

/** Authenticated auth endpoints (built on the main client). Login/logout are
 *  path-allowlisted in [com.mycloud.core.network.interceptor.AuthInterceptor]. */
interface AuthApi {

    @POST("api/v2/auth/login")
    suspend fun login(@Body body: LoginRequestDto): Response<Envelope<AuthTokensDto>>

    @POST("api/v2/auth/logout")
    suspend fun logout(@Body body: LogoutRequestDto): Response<Envelope<MessageDto>>

    @GET("api/v2/auth/me")
    suspend fun me(): Response<Envelope<UserDto>>

    @GET("api/v2/auth/whoami")
    suspend fun whoami(): Response<Envelope<WhoamiDto>>

    @POST("api/v2/auth/change-password")
    suspend fun changePassword(@Body body: ChangePasswordRequestDto): Response<Envelope<MessageDto>>

    /** Permanently deletes the caller's account and all owned resources (204). Retrofit
     *  forbids a body on a plain @DELETE, so this uses @HTTP with hasBody = true. */
    @HTTP(method = "DELETE", path = "api/v2/auth/account", hasBody = true)
    suspend fun deleteAccount(@Body body: DeleteAccountRequestDto): Response<Unit>
}
