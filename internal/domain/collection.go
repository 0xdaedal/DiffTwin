package domain

func RequestByID(collection *Collection, id string) *Request {
	if collection == nil {
		return nil
	}
	for i := range collection.Requests {
		if collection.Requests[i].ID == id {
			return &collection.Requests[i]
		}
	}
	return requestInFolders(collection.Folders, id)
}

func requestInFolders(folders []Folder, id string) *Request {
	for i := range folders {
		for j := range folders[i].Requests {
			if folders[i].Requests[j].ID == id {
				return &folders[i].Requests[j]
			}
		}
		if request := requestInFolders(folders[i].Folders, id); request != nil {
			return request
		}
	}
	return nil
}

func FolderByID(collection *Collection, id string) *Folder {
	if collection == nil || id == "" {
		return nil
	}
	return folderInFolders(collection.Folders, id)
}

func folderInFolders(folders []Folder, id string) *Folder {
	for i := range folders {
		if folders[i].ID == id {
			return &folders[i]
		}
		if folder := folderInFolders(folders[i].Folders, id); folder != nil {
			return folder
		}
	}
	return nil
}

func AppendFolder(folders *[]Folder, parentID string, folder Folder) bool {
	if parentID == "" {
		*folders = append(*folders, folder)
		return true
	}
	for i := range *folders {
		if (*folders)[i].ID == parentID {
			(*folders)[i].Folders = append((*folders)[i].Folders, folder)
			return true
		}
		if AppendFolder(&(*folders)[i].Folders, parentID, folder) {
			return true
		}
	}
	return false
}

func RemoveFolder(folders *[]Folder, id string) bool {
	for i := range *folders {
		if (*folders)[i].ID == id {
			*folders = append((*folders)[:i], (*folders)[i+1:]...)
			return true
		}
		if RemoveFolder(&(*folders)[i].Folders, id) {
			return true
		}
	}
	return false
}

func AppendRequestToFolder(collection *Collection, folderID string, request Request) bool {
	if folderID == "" {
		collection.Requests = append(collection.Requests, request)
		return true
	}
	folder := FolderByID(collection, folderID)
	if folder == nil {
		return false
	}
	folder.Requests = append(folder.Requests, request)
	return true
}

func UpdateRequest(collection *Collection, id string, update func(*Request)) bool {
	for i := range collection.Requests {
		if collection.Requests[i].ID == id {
			update(&collection.Requests[i])
			return true
		}
	}
	return updateRequestInFolders(collection.Folders, id, update)
}

func updateRequestInFolders(folders []Folder, id string, update func(*Request)) bool {
	for i := range folders {
		for j := range folders[i].Requests {
			if folders[i].Requests[j].ID == id {
				update(&folders[i].Requests[j])
				return true
			}
		}
		if updateRequestInFolders(folders[i].Folders, id, update) {
			return true
		}
	}
	return false
}

func RemoveRequest(collection *Collection, id string) bool {
	for i := range collection.Requests {
		if collection.Requests[i].ID == id {
			collection.Requests = append(collection.Requests[:i], collection.Requests[i+1:]...)
			return true
		}
	}
	return removeRequestFromFolders(collection.Folders, id)
}

func removeRequestFromFolders(folders []Folder, id string) bool {
	for i := range folders {
		for j := range folders[i].Requests {
			if folders[i].Requests[j].ID == id {
				folders[i].Requests = append(folders[i].Requests[:j], folders[i].Requests[j+1:]...)
				return true
			}
		}
		if removeRequestFromFolders(folders[i].Folders, id) {
			return true
		}
	}
	return false
}

func CountRequests(requests []Request, folders []Folder) int {
	count := len(requests)
	for _, folder := range folders {
		count += CountRequests(folder.Requests, folder.Folders)
	}
	return count
}
