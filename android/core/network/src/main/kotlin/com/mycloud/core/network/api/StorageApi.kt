package com.mycloud.core.network.api

import com.mycloud.core.network.dto.StorageStatsDto
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.GET

interface StorageApi {

    @GET("api/v2/storage/stats")
    suspend fun stats(): Response<Envelope<StorageStatsDto>>
}
