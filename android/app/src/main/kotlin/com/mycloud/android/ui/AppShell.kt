package com.mycloud.android.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.Folder
import com.mycloud.core.model.Permission

private enum class HomeDestination(val label: String, val icon: ImageVector) {
    FILES("My Files", Icons.Filled.Folder),
    PHOTOS("Photos", Icons.Filled.Photo),
    SHARED("Shared", Icons.Filled.Share),
    TRASH("Trash", Icons.Filled.Delete),
    SETTINGS("Settings", Icons.Filled.Settings),
}

/**
 * Maps a deep-link URI to a top-level tab. Recognises both the custom scheme host
 * (mycloud://photos) and an https path segment (https://host/photos). Returns null
 * for anything unrecognised, in which case the shell stays on its current tab.
 */
private fun destinationForDeepLink(uri: android.net.Uri): HomeDestination? =
    when (
        when (uri.scheme?.lowercase()) {
            "mycloud" -> uri.host ?: uri.lastPathSegment
            "http", "https" -> uri.pathSegments.firstOrNull()
            else -> uri.lastPathSegment
        }.orEmpty().lowercase()
    ) {
        "files", "home" -> HomeDestination.FILES
        "photos" -> HomeDestination.PHOTOS
        "shared" -> HomeDestination.SHARED
        "trash" -> HomeDestination.TRASH
        "settings" -> HomeDestination.SETTINGS
        else -> null
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
    onEnterMainShell: () -> Unit = {},
    deepLink: android.net.Uri? = null,
    onDeepLinkHandled: () -> Unit = {},
) {
    // Ask for the notification permission only once the user reaches the main
    // shell (post-login), not on cold start before onboarding.
    LaunchedEffect(Unit) { onEnterMainShell() }

    var current by rememberSaveable { mutableStateOf(HomeDestination.FILES) }
    // Re-tapping the already-selected tab bumps this; the visible screen scrolls to top.
    var scrollToTopTick by rememberSaveable { mutableIntStateOf(0) }

    // Route an incoming deep link (e.g. mycloud://photos) to its top-level tab,
    // then consume it so it doesn't re-fire on recomposition.
    LaunchedEffect(deepLink) {
        deepLink?.let { uri ->
            destinationForDeepLink(uri)?.let { current = it }
            onDeepLinkHandled()
        }
    }
    // Saveable so an in-flight share/detail survives process death & rotation.
    var shareTarget by rememberSaveable(stateSaver = FileNodeSaver) { mutableStateOf<FileNode?>(null) }
    var shareFolderTarget by rememberSaveable(stateSaver = FolderSaver) { mutableStateOf<Folder?>(null) }
    var detailTarget by rememberSaveable(stateSaver = FileNodeSaver) { mutableStateOf<FileNode?>(null) }

    Box(modifier = modifier.fillMaxSize()) {
        NavigationSuiteScaffold(
            navigationSuiteItems = {
            HomeDestination.entries.forEach { destination ->
                item(
                    selected = current == destination,
                    onClick = {
                        if (current == destination) scrollToTopTick++ else current = destination
                    },
                    icon = { Icon(destination.icon, contentDescription = destination.label) },
                    label = { Text(destination.label) },
                    modifier = Modifier.semantics { role = Role.Tab },
                )
            }
        },
    ) {
        Surface(modifier = Modifier.fillMaxSize().safeDrawingPadding()) {
            Crossfade(targetState = current, label = "destination") { dest ->
                when (dest) {
                    HomeDestination.FILES ->
                        com.mycloud.feature.browser.BrowserScreen(
                            onOpenFile = { detailTarget = it },
                            onShareFile = { shareTarget = it },
                            onShareFolder = { shareFolderTarget = it },
                            scrollToTopSignal = scrollToTopTick,
                        )
                    HomeDestination.PHOTOS ->
                        com.mycloud.feature.photos.PhotosScreen(scrollToTopSignal = scrollToTopTick)
                    HomeDestination.SHARED ->
                        com.mycloud.feature.sharing.SharedScreen(scrollToTopSignal = scrollToTopTick)
                    HomeDestination.TRASH ->
                        com.mycloud.feature.trash.TrashScreen(scrollToTopSignal = scrollToTopTick)
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

        // Slide+fade the detail overlay in/out (no cross-fade through an empty
        // composable). Keep the last target around so the screen stays rendered
        // during the exit animation.
        var lastDetail by remember { mutableStateOf<FileNode?>(null) }
        detailTarget?.let { lastDetail = it }
        AnimatedVisibility(
            visible = detailTarget != null,
            enter = slideInHorizontally(initialOffsetX = { it }) + fadeIn(),
            exit = slideOutHorizontally(targetOffsetX = { it }) + fadeOut(),
        ) {
            lastDetail?.let { target ->
                com.mycloud.feature.filedetail.FileDetailScreen(
                    file = target,
                    onBack = { detailTarget = null },
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }
    }
}

/** Persists a nullable [FileNode] across process death (all fields are Bundle-savable). */
private val FileNodeSaver: Saver<FileNode?, Any> = listSaver(
    save = { node ->
        if (node == null) emptyList() else listOf(
            node.id, node.name, node.sizeBytes, node.mimeType, node.folderId, node.isStarred,
            node.createdAtMillis, node.updatedAtMillis, node.ownerId, node.permission?.name, node.shared,
        )
    },
    restore = { list ->
        if (list.isEmpty()) {
            null
        } else {
            runCatching {
                FileNode(
                    id = list[0] as String,
                    name = list[1] as String,
                    sizeBytes = list[2] as Long,
                    mimeType = list[3] as String?,
                    folderId = list[4] as String?,
                    isStarred = list[5] as Boolean,
                    createdAtMillis = list[6] as Long,
                    updatedAtMillis = list[7] as Long,
                    ownerId = list[8] as String?,
                    permission = (list[9] as String?)?.toPermissionOrNull(),
                    shared = list[10] as Boolean,
                )
            }.getOrNull()
        }
    },
)

/** Persists a nullable [Folder] across process death. */
private val FolderSaver: Saver<Folder?, Any> = listSaver(
    save = { folder ->
        if (folder == null) emptyList() else listOf(
            folder.id, folder.name, folder.parentId, folder.createdAtMillis, folder.updatedAtMillis,
            folder.isFavorited, folder.ownerId, folder.permission?.name, folder.shared,
        )
    },
    restore = { list ->
        if (list.isEmpty()) {
            null
        } else {
            runCatching {
                Folder(
                    id = list[0] as String,
                    name = list[1] as String,
                    parentId = list[2] as String?,
                    createdAtMillis = list[3] as Long,
                    updatedAtMillis = list[4] as Long,
                    isFavorited = list[5] as Boolean,
                    ownerId = list[6] as String?,
                    permission = (list[7] as String?)?.toPermissionOrNull(),
                    shared = list[8] as Boolean,
                )
            }.getOrNull()
        }
    },
)

private fun String.toPermissionOrNull(): Permission? =
    runCatching { Permission.valueOf(this) }.getOrNull()

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
