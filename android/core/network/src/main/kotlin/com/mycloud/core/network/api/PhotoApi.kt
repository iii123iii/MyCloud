package com.mycloud.core.network.api

import com.mycloud.core.network.dto.PhotosResponse
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.GET
import retrofit2.http.Query

interface PhotoApi {

    @GET("api/v2/photos")
    suspend fun listPhotos(
        @Query("from") from: String? = null,
        @Query("to") to: String? = null,
    ): Response<Envelope<PhotosResponse>>
}
