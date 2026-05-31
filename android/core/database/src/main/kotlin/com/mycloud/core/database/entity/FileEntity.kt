package com.mycloud.core.database.entity

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * Cached file row. [sortIndex] encodes the server's listing order for the file's
 * [folderId] so the Paging `PagingSource` can `ORDER BY sortIndex` and reproduce
 * exactly what the API returned, independent of the sort column.
 */
@Entity(tableName = "files")
data class FileEntity(
    @PrimaryKey val id: String,
    val name: String,
    val sizeBytes: Long,
    val mimeType: String?,
    val folderId: String?,
    val isStarred: Boolean,
    val createdAtMillis: Long,
    val updatedAtMillis: Long,
    val ownerId: String?,
    val permission: String?,
    val shared: Boolean,
    val sortIndex: Int,
    val cachedAtMillis: Long,
)
