@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.feature.filedetail

import android.content.Intent
import android.text.format.DateUtils
import androidx.activity.compose.BackHandler
import androidx.core.content.FileProvider
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.media.PdfViewer
import com.mycloud.core.media.ZoomableImage
import com.mycloud.core.common.util.ByteFormatter
import com.mycloud.core.common.util.FileKind
import com.mycloud.core.common.util.MimeType
import com.mycloud.core.model.Comment
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.FileVersion
import java.io.File

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FileDetailScreen(
    file: FileNode,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: FileDetailViewModel = hiltViewModel(),
) {
    LaunchedEffect(file.id) { viewModel.load(file) }
    val state by viewModel.state.collectAsStateWithLifecycle()
    val pdfState by viewModel.pdf.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<Comment?>(null) }
    var pendingDelete by remember { mutableStateOf<String?>(null) }
    var showDiscardDialog by remember { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    // Confirm before throwing away an unsaved comment draft on back press.
    BackHandler(enabled = state.newComment.isNotBlank()) { showDiscardDialog = true }

    // Surface any transient comment/version/mutation error, then clear it.
    LaunchedEffect(state.errorMessage) {
        state.errorMessage?.let {
            snackbarHostState.showSnackbar(it)
            viewModel.clearError()
        }
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = { Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    IconButton(
                        onClick = {
                            viewModel.prepareLocalFile { localFile, mime ->
                                if (!shareFile(context, localFile, mime)) {
                                    scope.launch {
                                        snackbarHostState.showSnackbar("No app can share this file.")
                                    }
                                }
                            }
                        },
                        enabled = !state.isPreparingFile,
                    ) {
                        if (state.isPreparingFile) {
                            LoadingIndicator(modifier = Modifier.size(24.dp))
                        } else {
                            Icon(Icons.Filled.Share, contentDescription = "Share")
                        }
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            when (MimeType.kindOf(file.mimeType)) {
                FileKind.IMAGE ->
                    ZoomableImage(
                        model = viewModel.previewUrl(file.id),
                        contentDescription = file.name,
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(320.dp),
                    )
                FileKind.PDF -> when (val pdf = pdfState) {
                    is PdfState.Ready ->
                        PdfViewer(
                            file = pdf.file,
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(500.dp),
                        )
                    is PdfState.Loading ->
                        Column(
                            Modifier
                                .fillMaxWidth()
                                .height(200.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            LoadingIndicator()
                            Spacer(Modifier.height(12.dp))
                            Text(
                                "Downloading…",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    is PdfState.Error ->
                        Column(
                            Modifier
                                .fillMaxWidth()
                                .height(200.dp)
                                .clickable { viewModel.loadPdf() },
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            Text(
                                pdf.message,
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                            )
                            Spacer(Modifier.height(8.dp))
                            TextButton(onClick = { viewModel.loadPdf() }) { Text("Retry") }
                        }
                }
                else ->
                    Column(
                        Modifier
                            .fillMaxWidth()
                            .height(200.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.Center,
                    ) {
                        Icon(
                            Icons.Filled.InsertDriveFile,
                            contentDescription = "No preview available",
                            modifier = Modifier.size(64.dp),
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Spacer(Modifier.height(12.dp))
                        Button(
                            onClick = {
                                viewModel.prepareLocalFile { localFile, mime ->
                                    if (!openFile(context, localFile, mime)) {
                                        scope.launch {
                                            snackbarHostState.showSnackbar("No app can open this file.")
                                        }
                                    }
                                }
                            },
                            enabled = !state.isPreparingFile,
                        ) {
                            if (state.isPreparingFile) {
                                LoadingIndicator(modifier = Modifier.size(20.dp))
                            } else {
                                Icon(Icons.Filled.Download, contentDescription = null)
                                Spacer(Modifier.size(8.dp))
                                Text("Open / Download")
                            }
                        }
                    }
            }

            Text(file.name, style = MaterialTheme.typography.titleMedium)
            Text(
                ByteFormatter.format(file.sizeBytes),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                "Modified ${formatTimestamp(file.updatedAtMillis)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            HorizontalDivider()

            Text("Versions", style = MaterialTheme.typography.titleSmall)
            if (state.versions.isEmpty()) {
                Text(
                    "No previous versions.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                state.versions.forEach { version -> VersionRow(version, onRestore = { viewModel.restoreVersion(version.versionNumber) }) }
            }

            HorizontalDivider()

            Text("Comments", style = MaterialTheme.typography.titleSmall)
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = state.newComment,
                    onValueChange = { if (it.length <= MAX_COMMENT_LEN) viewModel.setNewComment(it) },
                    label = { Text("Add a comment") },
                    modifier = Modifier.weight(1f),
                    singleLine = true,
                )
                IconButton(
                    onClick = viewModel::addComment,
                    enabled = state.newComment.isNotBlank(),
                ) {
                    Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "Send")
                }
            }
            if (state.comments.isEmpty()) {
                Text(
                    "No comments yet.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                state.comments.forEach { comment ->
                    CommentRow(
                        comment,
                        onEdit = { editing = comment },
                        onDelete = { pendingDelete = comment.id },
                    )
                }
            }
        }
    }

    editing?.let { comment ->
        EditCommentDialog(
            initial = comment.content,
            onConfirm = { text ->
                viewModel.editComment(comment.id, text)
                editing = null
            },
            onDismiss = { editing = null },
        )
    }

    pendingDelete?.let { commentId ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text("Delete comment?") },
            text = { Text("This comment will be permanently removed.") },
            confirmButton = {
                TextButton(
                    onClick = {
                        viewModel.deleteComment(commentId)
                        pendingDelete = null
                    },
                ) { Text("Delete") }
            },
            dismissButton = { TextButton(onClick = { pendingDelete = null }) { Text("Cancel") } },
        )
    }

    if (showDiscardDialog) {
        AlertDialog(
            onDismissRequest = { showDiscardDialog = false },
            title = { Text("Discard comment?") },
            text = { Text("Your unsaved comment will be lost.") },
            confirmButton = {
                TextButton(
                    onClick = {
                        showDiscardDialog = false
                        onBack()
                    },
                ) { Text("Discard") }
            },
            dismissButton = { TextButton(onClick = { showDiscardDialog = false }) { Text("Keep") } },
        )
    }
}

/** Build a shareable content:// URI for [file] via the app's existing FileProvider. */
private fun fileProviderUri(context: android.content.Context, file: File) =
    FileProvider.getUriForFile(context, context.packageName + ".fileprovider", file)

/** Launch a chooser to share [file] of the given [mime] type via ACTION_SEND. */
private fun shareFile(context: android.content.Context, file: File, mime: String): Boolean {
    val uri = fileProviderUri(context, file)
    val send = Intent(Intent.ACTION_SEND).apply {
        type = mime
        putExtra(Intent.EXTRA_STREAM, uri)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    return runCatching {
        context.startActivity(Intent.createChooser(send, null))
        true
    }.getOrDefault(false)
}

/** Launch a chooser to open/view [file] of the given [mime] type via ACTION_VIEW. */
private fun openFile(context: android.content.Context, file: File, mime: String): Boolean {
    val uri = fileProviderUri(context, file)
    val view = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(uri, mime)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    return runCatching {
        context.startActivity(Intent.createChooser(view, null))
        true
    }.getOrDefault(false)
}

// Context-free localized date+time formatter. DateUtils.formatDateTime(null, …)
// NPEs on targetSdk 37+ because is24HourFormat now dereferences the (null)
// Context; java.time avoids needing a Context entirely (minSdk 26 supports it).
private val timestampFormatter: DateTimeFormatter =
    DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)

/** Absolute date+time for file/version timestamps (epoch millis). */
private fun formatTimestamp(millis: Long): String =
    Instant.ofEpochMilli(millis).atZone(ZoneId.systemDefault()).format(timestampFormatter)

/** Relative ("2 hours ago") time for comments (epoch millis). */
private fun formatRelative(millis: Long): String =
    DateUtils.getRelativeTimeSpanString(
        millis,
        System.currentTimeMillis(),
        DateUtils.MINUTE_IN_MILLIS,
    ).toString()

@Composable
private fun EditCommentDialog(
    initial: String,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var text by remember { mutableStateOf(initial) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Edit comment") },
        text = {
            OutlinedTextField(
                value = text,
                onValueChange = { if (it.length <= MAX_COMMENT_LEN) text = it },
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(text) },
                enabled = text.isNotBlank() && text.trim() != initial.trim(),
            ) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun VersionRow(version: FileVersion, onRestore: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(
                "v${version.versionNumber} • ${ByteFormatter.format(version.sizeBytes)}",
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                formatTimestamp(version.createdAtMillis),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        TextButton(onClick = onRestore) { Text("Restore") }
    }
}

@Composable
private fun CommentRow(comment: Comment, onEdit: () -> Unit, onDelete: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(
                "${comment.authorName ?: "User"} • ${formatRelative(comment.createdAtMillis)}",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(comment.content, style = MaterialTheme.typography.bodyMedium)
        }
        // Only the author may edit/delete (server-enforced via `editable`).
        if (comment.editable) {
            IconButton(onClick = onEdit) {
                Icon(Icons.Filled.Edit, contentDescription = "Edit comment")
            }
            IconButton(onClick = onDelete) {
                Icon(Icons.Filled.Delete, contentDescription = "Delete comment")
            }
        }
    }
}

/** Matches the backend's `maxCommentBody` so over-limit text is rejected before send. */
private const val MAX_COMMENT_LEN = 4000
