package com.mycloud.data.paging

import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import androidx.room.withTransaction
import com.mycloud.core.database.MyCloudDatabase
import com.mycloud.core.database.entity.FileEntity
import com.mycloud.core.database.entity.RemoteKeyEntity
import com.mycloud.core.model.SortKey
import com.mycloud.core.model.SortOrder
import com.mycloud.core.network.api.FileApi
import com.mycloud.data.mapper.toEntity
import retrofit2.HttpException
import java.io.IOException

/**
 * Cursor-backed Paging mediator. Room is the source of truth; this fills it from
 * `GET /files`. REFRESH clears the folder's rows + key and reloads page 1; APPEND
 * continues from the stored opaque cursor. Server order is preserved via
 * [FileEntity.sortIndex].
 */
@OptIn(ExperimentalPagingApi::class)
class FileRemoteMediator(
    private val folderId: String?,
    private val sort: SortKey,
    private val order: SortOrder,
    private val pageSize: Int,
    private val fileApi: FileApi,
    private val db: MyCloudDatabase,
) : RemoteMediator<Int, FileEntity>() {

    private val queryKey = "files:${folderId ?: "root"}:${sort.wire}:${order.wire}"

    override suspend fun load(
        loadType: LoadType,
        state: PagingState<Int, FileEntity>,
    ): MediatorResult {
        return try {
            val cursor: String? = when (loadType) {
                LoadType.REFRESH -> null
                LoadType.PREPEND -> return MediatorResult.Success(endOfPaginationReached = true)
                // The backend sends `next_cursor: ""` (empty, not null) on the last
                // page — treat blank as "no more pages", else APPEND loops forever
                // re-fetching page 1.
                LoadType.APPEND -> db.remoteKeyDao().get(queryKey)?.nextCursor?.takeIf { it.isNotBlank() }
                    ?: return MediatorResult.Success(endOfPaginationReached = true)
            }

            val response = fileApi.listFiles(folderId, sort.wire, order.wire, cursor, pageSize)
            if (!response.isSuccessful) {
                return MediatorResult.Error(HttpException(response))
            }
            val body = response.body()
            val files = body?.data?.files.orEmpty()
            // Normalise blank → null so end-of-pagination is detected correctly.
            val nextCursor = body?.meta?.nextCursor?.takeIf { it.isNotBlank() }

            db.withTransaction {
                if (loadType == LoadType.REFRESH) {
                    db.fileDao().clearFolder(folderId)
                    db.remoteKeyDao().delete(queryKey)
                }
                val start = if (loadType == LoadType.REFRESH) {
                    0
                } else {
                    (db.fileDao().maxSortIndex(folderId) ?: -1) + 1
                }
                val now = System.currentTimeMillis()
                db.fileDao().upsertAll(files.mapIndexed { i, dto -> dto.toEntity(start + i, now) })
                db.remoteKeyDao().upsert(RemoteKeyEntity(queryKey, nextCursor))
            }

            MediatorResult.Success(endOfPaginationReached = nextCursor == null)
        } catch (e: IOException) {
            MediatorResult.Error(e)
        } catch (e: HttpException) {
            MediatorResult.Error(e)
        }
    }
}
