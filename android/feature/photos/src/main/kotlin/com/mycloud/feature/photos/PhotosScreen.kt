package com.mycloud.feature.photos

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil3.compose.AsyncImage
import com.mycloud.core.media.ZoomableImage
import com.mycloud.core.model.Photo

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhotosScreen(
    modifier: Modifier = Modifier,
    scrollToTopSignal: Int = 0,
    viewModel: PhotosViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    // Survives rotation: persist the selected photo's id and re-derive the Photo.
    var selectedId by rememberSaveable { mutableStateOf<String?>(null) }
    val gridState = rememberLazyGridState()

    // Auto-refresh when the screen (re)appears (the VM is Activity-scoped).
    LaunchedEffect(Unit) { viewModel.refresh() }
    // Re-tapping the Photos tab scrolls back to the top.
    LaunchedEffect(scrollToTopSignal) {
        if (scrollToTopSignal > 0) gridState.animateScrollToItem(0)
    }

    PullToRefreshBox(
        isRefreshing = state.isLoading,
        onRefresh = viewModel::refresh,
        modifier = modifier
            .fillMaxSize()
            .statusBarsPadding(),
    ) {
        if (state.sections.isEmpty()) {
            // Scrollable container so PullToRefreshBox still gets the pull when empty.
            LazyColumn(Modifier.fillMaxSize()) {
                item {
                    Box(Modifier.fillParentMaxSize(), contentAlignment = Alignment.Center) {
                        Text(
                            when {
                                state.error != null -> state.error!!
                                state.isLoading -> "Loading photos…"
                                else -> "No photos yet."
                            },
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        } else {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(minSize = 100.dp),
                state = gridState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(4.dp),
            ) {
                state.sections.forEach { section ->
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Text(
                            section.title,
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 8.dp, vertical = 12.dp)
                                .semantics { heading() },
                        )
                    }
                    items(section.photos, key = { it.id }) { photo ->
                        PhotoTile(
                            url = viewModel.thumbnailUrl(photo.id),
                            name = photo.name,
                            onClick = { selectedId = photo.id },
                        )
                    }
                }
            }
        }
    }

    val selected = selectedId?.let { id ->
        state.sections.firstNotNullOfOrNull { section -> section.photos.firstOrNull { it.id == id } }
    }
    selected?.let { photo ->
        Dialog(
            onDismissRequest = { selectedId = null },
            properties = DialogProperties(usePlatformDefaultWidth = false),
        ) {
            Box(
                Modifier
                    .fillMaxSize()
                    .background(Color.Black)
                    .statusBarsPadding(),
            ) {
                ZoomableImage(
                    model = viewModel.previewUrl(photo.id),
                    contentDescription = photo.name,
                    modifier = Modifier.fillMaxSize(),
                )
                IconButton(
                    onClick = { selectedId = null },
                    modifier = Modifier
                        .align(Alignment.TopStart)
                        .padding(8.dp),
                ) {
                    Icon(Icons.Filled.Close, contentDescription = "Close", tint = Color.White)
                }
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun PhotoTile(url: String, name: String, onClick: () -> Unit) {
    val haptic = LocalHapticFeedback.current
    Box(
        Modifier
            .padding(2.dp)
            .aspectRatio(1f)
            .clip(RoundedCornerShape(4.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .combinedClickable(
                onClick = onClick,
                onLongClick = { haptic.performHapticFeedback(HapticFeedbackType.LongPress) },
            ),
    ) {
        AsyncImage(
            model = url,
            // A friendlier label than the raw filename (drop the extension).
            contentDescription = "Photo ${name.substringBeforeLast('.')}",
            contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize(),
        )
    }
}
