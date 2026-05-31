package com.mycloud.core.network.api

import com.mycloud.core.network.dto.VersionsResponse
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path

interface VersionApi {

    @GET("api/v2/files/{id}/versions")
    suspend fun listVersions(@Path("id") fileId: String): Response<Envelope<VersionsResponse>>

    @POST("api/v2/files/{id}/versions/{vno}:restore")
    suspend fun restoreVersion(
        @Path("id") fileId: String,
        @Path("vno") versionNumber: Int,
    ): Response<Unit>
}
