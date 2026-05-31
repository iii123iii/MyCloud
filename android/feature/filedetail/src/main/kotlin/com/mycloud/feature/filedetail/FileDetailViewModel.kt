package com.mycloud.feature.filedetail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.util.FileKind
import com.mycloud.core.common.util.MimeType
import com.mycloud.core.model.Comment
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.FileVersion
import com.mycloud.data.repository.CommentRepository
import com.mycloud.data.repository.FileRepository
import com.mycloud.data.repository.VersionRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File
import javax.inject.Inject

data class FileDetailUiState(
    val comments: List<Comment> = emptyList(),
    val versions: List<FileVersion> = emptyList(),
    val newComment: String = "",
    val isLoading: Boolean = false,
)

@HiltViewModel
class FileDetailViewModel @Inject constructor(
    private val commentRepository: CommentRepository,
    private val versionRepository: VersionRepository,
    private val fileRepository: FileRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(FileDetailUiState())
    val state: StateFlow<FileDetailUiState> = _state.asStateFlow()

    private val _pdf = MutableStateFlow<File?>(null)
    val pdf: StateFlow<File?> = _pdf.asStateFlow()

    private var fileId: String? = null
    private var pdfJob: Job? = null

    fun load(file: FileNode) {
        // Always (re)load on entry — LaunchedEffect(file.id) only fires this on a real
        // open, so re-opening a file shows fresh data and switching files never shows
        // the previous file's comments/versions/PDF.
        fileId = file.id
        pdfJob?.cancel()
        _pdf.value = null
        _state.value = FileDetailUiState(isLoading = true)
        refresh()
        if (MimeType.kindOf(file.mimeType) == FileKind.PDF) {
            pdfJob = viewModelScope.launch {
                val downloaded = fileRepository.downloadToCache(file.id, file.name)
                // Ignore a download that finished after the user already switched files.
                if (fileId == file.id) _pdf.value = downloaded
            }
        }
    }

    private fun refresh() {
        val id = fileId ?: return
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            val comments = (commentRepository.list(id) as? NetworkResult.Success)?.data.orEmpty()
            val versions = (versionRepository.list(id) as? NetworkResult.Success)?.data.orEmpty()
            _state.update { it.copy(comments = comments, versions = versions, isLoading = false) }
        }
    }

    fun setNewComment(value: String) = _state.update { it.copy(newComment = value) }

    fun addComment() {
        val id = fileId ?: return
        val text = _state.value.newComment.trim()
        if (text.isEmpty()) return
        viewModelScope.launch {
            commentRepository.add(id, text)
            _state.update { it.copy(newComment = "") }
            refresh()
        }
    }

    fun editComment(commentId: String, content: String) {
        val text = content.trim()
        if (text.isEmpty()) return
        viewModelScope.launch {
            commentRepository.edit(commentId, text)
            refresh()
        }
    }

    fun deleteComment(commentId: String) {
        viewModelScope.launch {
            commentRepository.delete(commentId)
            refresh()
        }
    }

    fun restoreVersion(versionNumber: Int) {
        val id = fileId ?: return
        viewModelScope.launch {
            versionRepository.restore(id, versionNumber)
            refresh()
        }
    }

    fun previewUrl(id: String): String = fileRepository.previewUrl(id)
}
