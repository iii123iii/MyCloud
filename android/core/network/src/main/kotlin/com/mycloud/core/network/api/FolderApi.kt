package com.mycloud.core.network.api

import com.mycloud.core.network.dto.CreateFolderRequestDto
import com.mycloud.core.network.dto.FolderDto
import com.mycloud.core.network.dto.FolderPathResponse
import com.mycloud.core.network.dto.FoldersResponse
import com.mycloud.core.network.dto.UpdateFolderRequestDto
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

interface FolderApi {

    @GET("api/v2/folders")
    suspend fun listFolders(
        @Query("parent_id") parentId: String?,
    ): Response<Envelope<FoldersResponse>>

    @GET("api/v2/folders/{id}/path")
    suspend fun folderPath(@Path("id") id: String): Response<Envelope<FolderPathResponse>>

    @POST("api/v2/folders")
    suspend fun createFolder(@Body body: CreateFolderRequestDto): Response<Envelope<FolderDto>>

    @PATCH("api/v2/folders/{id}")
    suspend fun updateFolder(
        @Path("id") id: String,
        @Body body: UpdateFolderRequestDto,
    ): Response<Envelope<FolderDto>>

    /** Move: PATCH with a raw `{"parent_id": "<dest>"|null}` body — an explicit
     *  `null` means "move to root", which the null-dropping global Json can't
     *  express through [UpdateFolderRequestDto]. */
    @PATCH("api/v2/folders/{id}")
    suspend fun moveFolder(@Path("id") id: String, @Body body: JsonObject): Response<Unit>

    /** Copy a subtree: POST with `{"parent_id": "<dest>"|null}`. Server replies 201 `{id}`. */
    @POST("api/v2/folders/{id}:copy")
    suspend fun copyFolder(@Path("id") id: String, @Body body: JsonObject): Response<Unit>

    @DELETE("api/v2/folders/{id}")
    suspend fun deleteFolder(@Path("id") id: String): Response<Unit>
}
