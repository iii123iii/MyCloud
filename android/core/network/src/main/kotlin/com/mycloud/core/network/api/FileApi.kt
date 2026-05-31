package com.mycloud.core.network.api

import com.mycloud.core.network.dto.FileDto
import com.mycloud.core.network.dto.FilesResponse
import com.mycloud.core.network.dto.UpdateFileRequestDto
import com.mycloud.core.network.envelope.Envelope
import kotlinx.serialization.json.JsonObject
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

interface FileApi {

    @GET("api/v2/files")
    suspend fun listFiles(
        @Query("folder_id") folderId: String?,
        @Query("sort") sort: String,
        @Query("order") order: String,
        @Query("cursor") cursor: String?,
        @Query("limit") limit: Int,
    ): Response<Envelope<FilesResponse>>

    @PATCH("api/v2/files/{id}")
    suspend fun updateFile(
        @Path("id") id: String,
        @Body body: UpdateFileRequestDto,
    ): Response<Envelope<FileDto>>

    /** Move: PATCH with a raw `{"folder_id": "<dest>"|null}` body. A JsonObject is
     *  used (not [UpdateFileRequestDto]) because the global Json drops nulls
     *  (`explicitNulls = false`), and the backend needs an explicit `null` to mean
     *  "move to root". Reply is `{message}`, so we read it as Unit. */
    @PATCH("api/v2/files/{id}")
    suspend fun moveFile(@Path("id") id: String, @Body body: JsonObject): Response<Unit>

    /** Copy: POST with `{"folder_id": "<dest>"|null}`. Server replies 201 `{id}`. */
    @POST("api/v2/files/{id}:copy")
    suspend fun copyFile(@Path("id") id: String, @Body body: JsonObject): Response<Unit>

    /** Soft-delete to trash. Server replies 204. */
    @DELETE("api/v2/files/{id}")
    suspend fun deleteFile(@Path("id") id: String): Response<Unit>
}
