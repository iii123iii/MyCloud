package com.mycloud.core.common.util

import java.util.Locale

/** Human-readable byte sizes (e.g. 1536 -> "1.5 KB"), matching the web's `formatBytes`. */
object ByteFormatter {

    private val units = arrayOf("B", "KB", "MB", "GB", "TB", "PB")

    fun format(bytes: Long, locale: Locale = Locale.getDefault()): String {
        if (bytes < 0) return "—"
        if (bytes < 1024) return "$bytes B"
        var value = bytes.toDouble()
        var unit = 0
        while (value >= 1024 && unit < units.lastIndex) {
            value /= 1024.0
            unit++
        }
        return String.format(locale, "%.1f %s", value, units[unit])
    }

    /**
     * Used / quota as a 0..1 fraction for quota bars; 0 when quota is unset/unknown
     * (<= 0) or used is unknown (< 0) — consistent with [format] treating negative
     * sizes as "unknown".
     */
    fun usedFraction(usedBytes: Long, quotaBytes: Long): Float =
        if (quotaBytes <= 0 || usedBytes < 0) {
            0f
        } else {
            (usedBytes.toDouble() / quotaBytes).coerceIn(0.0, 1.0).toFloat()
        }
}
