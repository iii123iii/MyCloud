package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

/** Body of POST /files:download-archive — a mixed selection zipped server-side.
 *  Field names map to file_ids / folder_ids via the global SnakeCase strategy. */
@Serializable
data class DownloadArchiveRequestDto(
    val fileIds: List<String> = emptyList(),
    val folderIds: List<String> = emptyList(),
)
