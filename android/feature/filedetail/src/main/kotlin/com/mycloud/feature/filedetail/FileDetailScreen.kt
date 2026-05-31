package com.mycloud.feature.filedetail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
    val pdfFile by viewModel.pdf.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<Comment?>(null) }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
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
                FileKind.PDF -> {
                    val pdf = pdfFile
                    if (pdf != null) {
                        PdfViewer(
                            file = pdf,
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(500.dp),
                        )
                    } else {
                        Box(
                            Modifier
                                .fillMaxWidth()
                                .height(200.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            CircularProgressIndicator()
                        }
                    }
                }
                else ->
                    Box(
                        Modifier
                            .fillMaxWidth()
                            .height(160.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        Icon(
                            Icons.Filled.InsertDriveFile,
                            contentDescription = null,
                            modifier = Modifier.size(64.dp),
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
            }

            Text(file.name, style = MaterialTheme.typography.titleMedium)
            Text(
                ByteFormatter.format(file.sizeBytes),
                style = MaterialTheme.typography.bodyMedium,
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
                        onDelete = { viewModel.deleteComment(comment.id) },
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
}

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
        Text(
            "v${version.versionNumber} • ${ByteFormatter.format(version.sizeBytes)}",
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium,
        )
        TextButton(onClick = onRestore) { Text("Restore") }
    }
}

@Composable
private fun CommentRow(comment: Comment, onEdit: () -> Unit, onDelete: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(
                comment.authorName ?: "User",
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
