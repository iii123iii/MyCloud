package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.model.Comment
import com.mycloud.core.network.api.CommentApi
import com.mycloud.core.network.dto.CreateCommentRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

interface CommentRepository {
    suspend fun list(fileId: String): NetworkResult<List<Comment>>
    suspend fun add(fileId: String, content: String): NetworkResult<Unit>
    suspend fun edit(commentId: String, content: String): NetworkResult<Unit>
    suspend fun delete(commentId: String): NetworkResult<Unit>
}

@Singleton
class CommentRepositoryImpl @Inject constructor(
    private val commentApi: CommentApi,
    private val json: Json,
) : CommentRepository {

    override suspend fun list(fileId: String): NetworkResult<List<Comment>> =
        safeApiCall(json) { commentApi.listComments(fileId) }
            .map { it.comments.map { dto -> dto.toDomain() } }

    override suspend fun add(fileId: String, content: String): NetworkResult<Unit> =
        safeApiCall(json) { commentApi.createComment(fileId, CreateCommentRequestDto(content)) }.map { }

    override suspend fun edit(commentId: String, content: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { commentApi.updateComment(commentId, CreateCommentRequestDto(content)) }

    override suspend fun delete(commentId: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { commentApi.deleteComment(commentId) }
}
