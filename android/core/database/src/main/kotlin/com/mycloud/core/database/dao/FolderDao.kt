package com.mycloud.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import com.mycloud.core.database.entity.FolderEntity
import kotlinx.coroutines.flow.Flow

@Dao
interface FolderDao {

    @Query(
        "SELECT * FROM folders WHERE (:parentId IS NULL AND parentId IS NULL) OR parentId = :parentId " +
            "ORDER BY name COLLATE NOCASE ASC",
    )
    fun observeChildren(parentId: String?): Flow<List<FolderEntity>>

    @Upsert
    suspend fun upsertAll(items: List<FolderEntity>)

    @Query("DELETE FROM folders WHERE (:parentId IS NULL AND parentId IS NULL) OR parentId = :parentId")
    suspend fun clearChildren(parentId: String?)

    @Query("DELETE FROM folders WHERE id = :id")
    suspend fun deleteById(id: String)
}
