package com.mycloud.core.network.api

import com.mycloud.core.network.dto.TrashResponse
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

interface TrashApi {

    @GET("api/v2/trash")
    suspend fun listTrash(
        @Query("limit") limit: Int = 100,
        @Query("offset") offset: Int = 0,
    ): Response<Envelope<TrashResponse>>

    @POST("api/v2/trash/{id}:restore")
    suspend fun restore(@Path("id") id: String): Response<Unit>

    @DELETE("api/v2/trash/{id}")
    suspend fun deleteForever(@Path("id") id: String): Response<Unit>

    @DELETE("api/v2/trash")
    suspend fun empty(): Response<Unit>
}
