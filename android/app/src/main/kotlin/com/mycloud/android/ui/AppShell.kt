package com.mycloud.android.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.Crossfade
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Photo
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.navigationsuite.ExperimentalMaterial3AdaptiveNavigationSuiteApi
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.Folder

private enum class HomeDestination(val label: String, val icon: ImageVector) {
    FILES("My Files", Icons.Filled.Folder),
    PHOTOS("Photos", Icons.Filled.Photo),
    SHARED("Shared", Icons.Filled.Share),
    TRASH("Trash", Icons.Filled.Delete),
    SETTINGS("Settings", Icons.Filled.Settings),
}

/**
 * The authenticated app shell. `NavigationSuiteScaffold` adapts the primary
 * navigation across window sizes (bottom bar → rail → drawer). Destination
 * content is placeholder pending the feature phases; this establishes the shell.
 */
@OptIn(ExperimentalMaterial3AdaptiveNavigationSuiteApi::class)
@Composable
fun AppShell(
    onSignOut: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var current by rememberSaveable { mutableStateOf(HomeDestination.FILES) }
    var shareTarget by remember { mutableStateOf<FileNode?>(null) }
    var shareFolderTarget by remember { mutableStateOf<Folder?>(null) }
    var detailTarget by remember { mutableStateOf<FileNode?>(null) }

    Box(modifier = modifier.fillMaxSize()) {
        NavigationSuiteScaffold(
            navigationSuiteItems = {
            HomeDestination.entries.forEach { destination ->
                item(
                    selected = current == destination,
                    onClick = { current = destination },
                    icon = { Icon(destination.icon, contentDescription = destination.label) },
                    label = { Text(destination.label) },
                )
            }
        },
    ) {
        Surface(modifier = Modifier.fillMaxSize()) {
            Crossfade(targetState = current, label = "destination") { dest ->
                when (dest) {
                    HomeDestination.FILES ->
                        com.mycloud.feature.browser.BrowserScreen(
                            onOpenFile = { detailTarget = it },
                            onShareFile = { shareTarget = it },
                            onShareFolder = { shareFolderTarget = it },
                        )
                    HomeDestination.PHOTOS ->
                        com.mycloud.feature.photos.PhotosScreen()
                    HomeDestination.SHARED ->
                        com.mycloud.feature.sharing.SharedScreen()
                    HomeDestination.TRASH ->
                        com.mycloud.feature.trash.TrashScreen()
                    HomeDestination.SETTINGS ->
                        SettingsScreen(onSignOut = onSignOut)
                }
            }
        }
            shareTarget?.let { file ->
                com.mycloud.feature.sharing.ShareDialog(
                    fileId = file.id,
                    folderId = null,
                    title = file.name,
                    onDismiss = { shareTarget = null },
                )
            }
            shareFolderTarget?.let { folder ->
                com.mycloud.feature.sharing.ShareDialog(
                    fileId = null,
                    folderId = folder.id,
                    title = folder.name,
                    onDismiss = { shareFolderTarget = null },
                )
            }
        }

        AnimatedContent(targetState = detailTarget, label = "detail") { target ->
            if (target != null) {
                com.mycloud.feature.filedetail.FileDetailScreen(
                    file = target,
                    onBack = { detailTarget = null },
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }
    }
}

@Composable
private fun Placeholder(title: String, subtitle: String) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(title, style = MaterialTheme.typography.headlineSmall)
        Spacer(Modifier.height(8.dp))
        Text(
            subtitle,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
