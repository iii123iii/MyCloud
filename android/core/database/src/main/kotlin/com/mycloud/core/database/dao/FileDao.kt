package com.mycloud.core.database.dao

import androidx.paging.PagingSource
import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import com.mycloud.core.database.entity.FileEntity

@Dao
interface FileDao {

    @Query(
        "SELECT * FROM files WHERE (:folderId IS NULL AND folderId IS NULL) OR folderId = :folderId " +
            "ORDER BY sortIndex ASC",
    )
    fun pagingSource(folderId: String?): PagingSource<Int, FileEntity>

    @Upsert
    suspend fun upsertAll(items: List<FileEntity>)

    @Query("DELETE FROM files WHERE (:folderId IS NULL AND folderId IS NULL) OR folderId = :folderId")
    suspend fun clearFolder(folderId: String?)

    @Query("SELECT MAX(sortIndex) FROM files WHERE (:folderId IS NULL AND folderId IS NULL) OR folderId = :folderId")
    suspend fun maxSortIndex(folderId: String?): Int?

    @Query("DELETE FROM files WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("UPDATE files SET isStarred = :starred WHERE id = :id")
    suspend fun setStarred(id: String, starred: Boolean)

    @Query("UPDATE files SET name = :name WHERE id = :id")
    suspend fun rename(id: String, name: String)
}
