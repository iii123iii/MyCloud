package com.mycloud.core.network.envelope

import com.mycloud.core.common.result.Meta
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

/**
 * The backend's uniform response envelope. Every endpoint returns either
 * `{data, meta}` (success) or `{error:{code,message,details}}` (failure), so a
 * single generic type deserializes them all.
 */
@Serializable
data class Envelope<T>(
    val data: T? = null,
    val meta: MetaDto? = null,
    val error: ErrorBody? = null,
)

@Serializable
data class MetaDto(
    @SerialName("next_cursor") val nextCursor: String? = null,
    val total: Int? = null,
) {
    fun toMeta(): Meta = Meta(nextCursor = nextCursor, total = total)
}

@Serializable
data class ErrorBody(
    val code: String = "",
    val message: String = "",
    val details: JsonElement? = null,
)
