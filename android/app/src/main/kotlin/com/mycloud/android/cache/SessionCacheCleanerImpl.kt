package com.mycloud.android.cache

import android.content.Context
import coil3.SingletonImageLoader
import com.mycloud.core.database.MyCloudDatabase
import com.mycloud.data.session.SessionCacheCleaner
import dagger.Lazy
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Clears every local cache on sign-out / user-switch:
 *  - Room cache (all tables) — the encrypted DB rows.
 *  - Coil image cache (memory + disk) — thumbnails/previews are file contents.
 *  - The download/preview scratch dir used by `FileRepository.downloadToCache`.
 *
 * Each step is independent (`runCatching`) so one failure can't leave the rest
 * un-wiped. Runs off the main thread.
 */
@Singleton
class SessionCacheCleanerImpl @Inject constructor(
    @ApplicationContext private val context: Context,
    // Lazy so wiring this into the auth layer doesn't construct (or key-init) the
    // encrypted DB at app start — it's built on first browse or first wipe.
    private val db: Lazy<MyCloudDatabase>,
) : SessionCacheCleaner {

    override suspend fun clear() {
        withContext(Dispatchers.IO) {
            runCatching { db.get().clearAllTables() }
            runCatching {
                val loader = SingletonImageLoader.get(context)
                loader.memoryCache?.clear()
                loader.diskCache?.clear()
            }
            // Belt-and-suspenders: drop the on-disk cache dirs by path too, in case
            // the loader's references differ from the default locations.
            runCatching { File(context.cacheDir, "image_cache").deleteRecursively() }
            runCatching { File(context.cacheDir, "previews").deleteRecursively() }
            // Pending archive-download selections (written under filesDir by the
            // transfer layer) — drop them so a signed-out session leaves nothing.
            runCatching { File(context.filesDir, "archive_requests").deleteRecursively() }
        }
    }
}
