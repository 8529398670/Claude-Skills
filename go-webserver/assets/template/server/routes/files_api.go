// whole-file feature:files
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for files.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListMyFiles , apidoc.Op{
	Summary: "Your files, newest first",
	Tag: "Files",
	Feature: "files",
	Params: listMyFilesParamsDoc{},
	Reply: listMyFilesReplyDoc{},
	Errors: []int{ 400 , 401 , 404 },
} )

type listMyFilesParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep files whose name contains this, in any case."`
	Sort string `query:"sort" enum:"name|size|created_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listMyFilesReplyDoc struct {
	Files []fileSchema `json:"files"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateFile , apidoc.Op{
	Summary: "Start an upload",
	Description: "Then send each piece in order and finish. A file over files.max_file_bytes, or one that would pass files.quota_bytes_per_user, is a 409.",
	Tag: "Files",
	Feature: "files",
	Body: createFileBodyDoc{},
	Status: 201,
	Reply: createFileReplyDoc{},
	ReplyDescription: "Created",
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type createFileBodyDoc struct {
	Name string `json:"name"`
	Size int `json:"size" minimum:"0"`
	Type string `json:"type,omitempty" doc:"Shown in the list. Downloads are always application/octet-stream."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type createFileReplyDoc struct {
	fileSchema
	ChunkBytes int `json:"chunk_bytes" doc:"Every piece but the last is exactly this long."`
}

var _ = apidoc.Doc( (*Handlers).GetMyFile , apidoc.Op{
	Summary: "One of your files",
	Description: "received is where to resume an upload.",
	Tag: "Files",
	Feature: "files",
	Reply: getMyFileReplyDoc{},
	Errors: []int{ 401 , 404 },
} )

type getMyFileReplyDoc struct {
	fileSchema
	ChunkBytes int `json:"chunk_bytes" doc:"Every piece but the last is exactly this long."`
}

var _ = apidoc.Doc( (*Handlers).PutFileChunk , apidoc.Op{
	Summary: "Send the next piece of an upload",
	Description: "The body is the raw bytes. Pieces go in order, from 0. A browser session sends its csrf token in the X-CSRF-Token header. A 409 carries received, the piece to send next. Counts against server.rate_limit_upload_max.",
	Tag: "Files",
	Feature: "files",
	MCPHTTP: "File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says where to send each piece, with the same key.",
	Params: putFileChunkParamsDoc{},
	BodyType: "application/octet-stream",
	Reply: putFileChunkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type putFileChunkParamsDoc struct {
	Index int `path:"index" minimum:"0"`
	XCSRFToken string `header:"X-CSRF-Token" doc:"From GET /api/me. Browser sessions only."`
}

type putFileChunkReplyDoc struct {
	Received int `json:"received"`
}

var _ = apidoc.Doc( (*Handlers).FinishFile , apidoc.Op{
	Summary: "Finish an upload",
	Tag: "Files",
	Feature: "files",
	Body: finishFileBodyDoc{},
	Reply: fileSchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type finishFileBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).DownloadMyFile , apidoc.Op{
	Summary: "Download one of your files",
	Tag: "Files",
	Feature: "files",
	ReplyType: "application/octet-stream",
	ReplyDescription: "The file, always as an attachment",
	Errors: []int{ 401 , 404 },
} )

var _ = apidoc.Doc( (*Handlers).RenameMyFile , apidoc.Op{
	Summary: "Rename one of your files",
	Tag: "Files",
	Feature: "files",
	Body: renameMyFileBodyDoc{},
	Reply: renameMyFileReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameMyFileBodyDoc struct {
	Name string `json:"name" doc:"Cleaned like an upload's name. 1-255 bytes."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameMyFileReplyDoc struct {
	Ok bool `json:"ok"`
	Name string `json:"name" doc:"As stored."`
}

var _ = apidoc.Doc( (*Handlers).DeleteMyFile , apidoc.Op{
	Summary: "Delete one of your files",
	Description: "It can be restored for ten minutes.",
	Tag: "Files",
	Feature: "files",
	Body: deleteMyFileBodyDoc{},
	Reply: deleteMyFileReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteMyFileBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteMyFileReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RestoreMyFile , apidoc.Op{
	Summary: "Put back a file you deleted",
	Tag: "Files",
	Feature: "files",
	Body: restoreMyFileBodyDoc{},
	Reply: fileSchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type restoreMyFileBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).ListAllFiles , apidoc.Op{
	Summary: "Every account's files",
	Tag: "Files",
	Role: apidoc.Admin,
	Feature: "files",
	Params: listAllFilesParamsDoc{},
	Reply: listAllFilesReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type listAllFilesParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep files whose name or owner contains this, in any case."`
	Sort string `query:"sort" enum:"name|user_name|size|created_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listAllFilesReplyDoc struct {
	Files []listAllFilesReplyDocFilesItem `json:"files"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

type listAllFilesReplyDocFilesItem struct {
	fileSchema
	UserID int `json:"user_id"`
	UserName string `json:"user_name"`
}

var _ = apidoc.Doc( (*Handlers).DownloadAnyFile , apidoc.Op{
	Summary: "Download anyone's file",
	Description: "Written to the activity log when the file is someone else's.",
	Tag: "Files",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "files",
	ReplyType: "application/octet-stream",
	ReplyDescription: "The file, always as an attachment",
	Errors: []int{ 401 , 403 , 404 },
} )

var _ = apidoc.Doc( (*Handlers).RenameAnyFile , apidoc.Op{
	Summary: "Rename any file",
	Tag: "Files",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "files",
	Body: renameAnyFileBodyDoc{},
	Reply: renameAnyFileReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameAnyFileBodyDoc struct {
	Name string `json:"name" doc:"Cleaned like an upload's name. 1-255 bytes."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameAnyFileReplyDoc struct {
	Ok bool `json:"ok"`
	Name string `json:"name" doc:"As stored."`
}

var _ = apidoc.Doc( (*Handlers).DeleteAnyFile , apidoc.Op{
	Summary: "Delete anyone's file",
	Tag: "Files",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "files",
	Body: deleteAnyFileBodyDoc{},
	Reply: deleteAnyFileReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteAnyFileBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteAnyFileReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RestoreAnyFile , apidoc.Op{
	Summary: "Put back a deleted file",
	Tag: "Files",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "files",
	Body: restoreAnyFileBodyDoc{},
	Reply: fileSchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type restoreAnyFileBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).MyStorage , apidoc.Op{
	Summary: "How much of your quota is used",
	Tag: "Files",
	Feature: "files",
	Reply: myStorageReplyDoc{},
	Errors: []int{ 401 , 404 },
} )

type myStorageReplyDoc struct {
	UsedBytes int `json:"used_bytes"`
	QuotaBytes int `json:"quota_bytes"`
}

var _ = apidoc.Doc( (*Handlers).AllStorage , apidoc.Op{
	Summary: "Every account's files together, and room on the disk",
	Tag: "Files",
	Role: apidoc.Admin,
	Feature: "files",
	Reply: allStorageReplyDoc{},
	Errors: []int{ 401 , 403 , 404 },
} )

type allStorageReplyDoc struct {
	UsedBytes int `json:"used_bytes"`
	DiskFreeBytes int `json:"disk_free_bytes,omitempty" doc:"Left out where the system will not say."`
	DiskTotalBytes int `json:"disk_total_bytes,omitempty" doc:"Left out where the system will not say."`
}
