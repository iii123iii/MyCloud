package com.mycloud.core.work

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.pm.ServiceInfo
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.work.ForegroundInfo
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.UUID
import javax.inject.Inject
import javax.inject.Singleton

/** Builds the foreground/progress notifications for transfer workers. */
@Singleton
class TransferNotifications @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    init {
        createChannel()
    }

    fun foregroundInfo(workId: UUID, name: String, progress: Int, upload: Boolean): ForegroundInfo {
        val notification = build(name, progress, ongoing = true, upload = upload)
        val id = workId.hashCode()
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ForegroundInfo(id, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            ForegroundInfo(id, notification)
        }
    }

    fun notifyProgress(notificationId: Int, name: String, progress: Int, upload: Boolean) {
        notify(notificationId, build(name, progress, ongoing = true, upload = upload))
    }

    /**
     * Time-throttled progress mutator for tight per-byte/per-percent loops: posts a
     * progress notification at most once per [PROGRESS_THROTTLE_MS] (always letting
     * 100% through). [lastEmitMs] is the caller's running timestamp; returns the
     * timestamp to keep for the next call.
     */
    fun notifyProgressThrottled(
        notificationId: Int,
        name: String,
        progress: Int,
        upload: Boolean,
        lastEmitMs: Long,
    ): Long {
        val now = System.currentTimeMillis()
        if (progress >= 100 || now - lastEmitMs >= PROGRESS_THROTTLE_MS) {
            notifyProgress(notificationId, name, progress, upload)
            return now
        }
        return lastEmitMs
    }

    fun notifyComplete(notificationId: Int, name: String, success: Boolean, upload: Boolean) {
        val verb = if (upload) "Upload" else "Download"
        val text = if (success) "$verb complete" else "$verb failed"
        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setContentTitle(name)
            .setContentText(text)
            .setOngoing(false)
            .setAutoCancel(true)
            .build()
        notify(notificationId, notification)
    }

    private fun build(name: String, progress: Int, ongoing: Boolean, upload: Boolean): Notification {
        val icon = if (upload) android.R.drawable.stat_sys_upload else android.R.drawable.stat_sys_download
        val title = if (upload) "Uploading" else "Downloading"
        return NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(icon)
            .setContentTitle(title)
            .setContentText(name)
            .setOngoing(ongoing)
            .setOnlyAlertOnce(true)
            .setProgress(100, progress.coerceIn(0, 100), progress <= 0)
            .build()
    }

    private fun notify(id: Int, notification: Notification) {
        // No-ops if POST_NOTIFICATIONS isn't granted (API 33+); the foreground
        // notification shown via setForeground is unaffected.
        runCatching { NotificationManagerCompat.from(context).notify(id, notification) }
    }

    private fun createChannel() {
        // NotificationChannel is API 26+ (O); on older releases channels don't exist.
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Transfers",
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = "Upload and download progress" }
        manager.createNotificationChannel(channel)
    }

    private companion object {
        const val CHANNEL_ID = "transfers"
        const val PROGRESS_THROTTLE_MS = 500L
    }
}
