@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.core.media

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * Renders a local PDF [file] page-by-page (via the platform [PdfRenderer]) inside
 * a [LazyColumn]. Each page is rasterized off the main thread only when its row is
 * composed, and the bitmap is recycled when the row scrolls off-screen — so a large
 * document never holds every page in memory at once.
 *
 * On a failure to open the document an error message is shown instead of a blank
 * (previously errors were swallowed into an empty page list, yielding a blank view).
 */
@Composable
fun PdfViewer(file: File, modifier: Modifier = Modifier) {
    // Cheap up-front step: open the document and read just the page count + per-page
    // aspect ratios. Heavy rasterization happens lazily, per visible row.
    val docInfo by produceState<PdfDocInfo?>(initialValue = null, file) {
        value = withContext(Dispatchers.IO) { readPdfInfo(file) }
    }

    when (val info = docInfo) {
        null ->
            Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                LoadingIndicator()
            }
        is PdfDocInfo.Failure ->
            Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    "Couldn't open this document.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        is PdfDocInfo.Pages ->
            LazyColumn(modifier.fillMaxSize()) {
                items(info.aspectRatios.size) { index ->
                    PdfPage(file = file, index = index, aspectRatio = info.aspectRatios[index])
                }
            }
    }
}

/** Renders a single page lazily and recycles its bitmap when this row leaves composition. */
@Composable
private fun PdfPage(file: File, index: Int, aspectRatio: Float) {
    val bitmap by produceState<Bitmap?>(initialValue = null, file, index) {
        value = withContext(Dispatchers.IO) { renderPage(file, index) }
    }

    // Reserve the page's footprint up front so scrolling doesn't jump as pages render.
    val pageModifier = Modifier
        .fillMaxWidth()
        .aspectRatio(aspectRatio)
        .padding(vertical = 4.dp)
        .background(androidx.compose.ui.graphics.Color.White)

    val rendered = bitmap
    if (rendered == null) {
        Box(pageModifier, contentAlignment = Alignment.Center) {
            LoadingIndicator()
        }
    } else {
        Image(
            bitmap = rendered.asImageBitmap(),
            contentDescription = null,
            contentScale = ContentScale.FillWidth,
            modifier = pageModifier,
        )
    }

    // Free the (potentially large) bitmap once the row scrolls off-screen / disposes.
    DisposableEffect(rendered) {
        onDispose { rendered?.recycle() }
    }
}

private const val TARGET_WIDTH_PX = 1080

private sealed interface PdfDocInfo {
    data class Pages(val aspectRatios: List<Float>) : PdfDocInfo
    data object Failure : PdfDocInfo
}

/** Opens the PDF once to learn the page count and each page's height/width ratio. */
private fun readPdfInfo(file: File): PdfDocInfo = runCatching {
    ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { pfd ->
        PdfRenderer(pfd).use { renderer ->
            val ratios = (0 until renderer.pageCount).map { index ->
                renderer.openPage(index).use { page ->
                    (page.height.toFloat() / page.width).takeIf { it > 0f } ?: 1f
                }
            }
            PdfDocInfo.Pages(ratios)
        }
    }
}.getOrElse { PdfDocInfo.Failure }

/** Rasterizes a single page to a bitmap, or null on failure. */
private fun renderPage(file: File, index: Int): Bitmap? = runCatching {
    ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { pfd ->
        PdfRenderer(pfd).use { renderer ->
            renderer.openPage(index).use { page ->
                val width = TARGET_WIDTH_PX
                val height = (page.height.toFloat() / page.width * width).toInt().coerceAtLeast(1)
                Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888).also { bmp ->
                    bmp.eraseColor(Color.WHITE)
                    page.render(bmp, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                }
            }
        }
    }
}.getOrNull()
