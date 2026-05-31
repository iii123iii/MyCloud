package com.mycloud.core.network.api

import com.mycloud.core.network.dto.CommentDto
import com.mycloud.core.network.dto.CommentsResponse
import com.mycloud.core.network.dto.CreateCommentRequestDto
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path

interface CommentApi {

    @GET("api/v2/files/{id}/comments")
    suspend fun listComments(@Path("id") fileId: String): Response<Envelope<CommentsResponse>>

    @POST("api/v2/files/{id}/comments")
    suspend fun createComment(
        @Path("id") fileId: String,
        @Body body: CreateCommentRequestDto,
    ): Response<Envelope<CommentDto>>

    @PATCH("api/v2/comments/{id}")
    suspend fun updateComment(
        @Path("id") commentId: String,
        @Body body: CreateCommentRequestDto,
    ): Response<Unit>

    @DELETE("api/v2/comments/{id}")
    suspend fun deleteComment(@Path("id") commentId: String): Response<Unit>
}
