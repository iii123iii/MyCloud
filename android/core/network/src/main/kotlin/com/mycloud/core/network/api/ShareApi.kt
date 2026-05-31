package com.mycloud.core.network.api

import com.mycloud.core.network.dto.CreateGrantRequestDto
import com.mycloud.core.network.dto.CreateShareRequestDto
import com.mycloud.core.network.dto.GrantDto
import com.mycloud.core.network.dto.GrantsResponse
import com.mycloud.core.network.dto.ShareDto
import com.mycloud.core.network.dto.SharesResponse
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

interface ShareApi {

    @GET("api/v2/shares")
    suspend fun listShares(
        @Query("limit") limit: Int = 100,
        @Query("offset") offset: Int = 0,
    ): Response<Envelope<SharesResponse>>

    @POST("api/v2/shares")
    suspend fun createShare(@Body body: CreateShareRequestDto): Response<Envelope<ShareDto>>

    @DELETE("api/v2/shares/{id}")
    suspend fun deleteShare(@Path("id") id: String): Response<Unit>

    // direction is REQUIRED in spirit: the backend defaults to "incoming" when it's
    // absent, which is the opposite of what the "Shared by me" screen wants.
    @GET("api/v2/shares/grants")
    suspend fun listGrants(
        @Query("direction") direction: String = "outgoing",
        @Query("limit") limit: Int = 100,
        @Query("offset") offset: Int = 0,
    ): Response<Envelope<GrantsResponse>>

    @POST("api/v2/shares/grants")
    suspend fun createGrant(@Body body: CreateGrantRequestDto): Response<Envelope<GrantDto>>

    @DELETE("api/v2/shares/grants/{id}")
    suspend fun deleteGrant(@Path("id") id: String): Response<Unit>
}
