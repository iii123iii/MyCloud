package com.mycloud.core.media

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.Composable
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
 * Renders a local PDF [file] page-by-page (via the platform [PdfRenderer]) and
 * shows the pages in a scrollable column. Pages are rasterized off the main
 * thread. Suitable for typical documents; very large PDFs are a memory caveat
 * (lazy per-page rendering is a future refinement).
 */
@Composable
fun PdfViewer(file: File, modifier: Modifier = Modifier) {
    val pages by produceState<List<Bitmap>?>(initialValue = null, file) {
        value = withContext(Dispatchers.IO) { renderPdf(file) }
    }

    val rendered = pages
    if (rendered == null) {
        Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
    } else {
        LazyColumn(modifier.fillMaxSize()) {
            items(rendered) { bitmap ->
                Image(
                    bitmap = bitmap.asImageBitmap(),
                    contentDescription = null,
                    contentScale = ContentScale.FillWidth,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 4.dp)
                        .background(androidx.compose.ui.graphics.Color.White),
                )
            }
        }
    }
}

private const val TARGET_WIDTH_PX = 1080

private fun renderPdf(file: File): List<Bitmap> = runCatching {
    ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { pfd ->
        PdfRenderer(pfd).use { renderer ->
            (0 until renderer.pageCount).map { index ->
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
    }
}.getOrDefault(emptyList())
