package main

import (
   
    "github.com/astaxie/beego/logs"
	"io"
	"strconv"
	"net/http"
	"time"
	"io/ioutil"
	"strings"
	"os"
	"path/filepath"
	"fmt"
	"os/exec"
	// "archive/tar"
	// "compress/gzip"
)

func CompareJSONFile(local map[string]interface{}, remote map[string]interface{}) (result map[string]interface{}){
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	mapData := make(map[string]string)
	mapData["Date"] = currentTime
	
	if local == nil {mapData["status"] = "Local Json doesn't exist"; Logger(mapData)}
	if remote == nil {mapData["status"] = "Remote Json doesn't exist"; Logger(mapData)}

	for k, v := range remote {
		switch vv := v.(type) {
		case string, bool:
			if local[k] == nil {
				mapData["date"] = currentTime
				mapData["type"] = "file"
				// mapData["localFile"] = fileName
				mapData["param"] = k
				mapData["currentValue"] = "nil"
				mapData["newValue"] = fmt.Sprintf("%s", remote[k])				
				local[k] = remote[k]
				Logger(mapData)

			}else{

				if vv != local[k]{
					mapData["date"] = currentTime
					mapData["type"] = "file"
					// mapData["localFile"] = fileName
					mapData["param"] = k
					mapData["currentValue"] = fmt.Sprintf("%s", local[k])
					mapData["newValue"] = fmt.Sprintf("%s", remote[k])
					Logger(mapData)
					
				}
			}
		case map[string]interface{}:
			if local[k] != nil {
				local[k] = CompareJSONFile(local[k].(map[string]interface{}), remote[k].(map[string]interface{}))				
			}else{
				mapData["date"] = currentTime
				mapData["type"] = "file"
				// mapData["localFile"] = fileName
				mapData["param"] = k
				mapData["currentValue"] = "nil"
				mapData["newValue"] = fmt.Sprintf("%s", remote[k])
				Logger(mapData)

				local[k] = remote[k]
			}
			
		default:

		}
	}

	return local
}

func CopyFiles(src string, dst string) (err error){
	logs.Info("SRC: "+src+ " -- DST: "+dst)
	_, err = exec.Command("bash","-c","cp -rp "+src+" "+dst).Output()
	if err != nil {logs.Error("CopyFiles Error copy: "+err.Error()); return err}
	return nil
}

func ExtractTarGz(tarGzFile string, pathDownloads string)(err error){
	if _, err := os.Stat(tarGzFile); os.IsNotExist(err) {
		logs.Error("TarGZ don't exist: "+err.Error())
		return err
	}

	if _, err := os.Stat(pathDownloads); os.IsNotExist(err) {
		err := os.MkdirAll(pathDownloads, 0755);
		if err != nil {
			logs.Error("Error creating folder for extract: "+err.Error())
			return err
		}
	}	

	_, err = exec.Command("bash","-c","tar -C "+pathDownloads+" -xzvf "+tarGzFile).Output()
	return err
}

func DownloadCurrentVersion(){
	err := DownloadFile(config.Tmpfolder+config.Versionfile, config.Repourl+config.Versionfile)
	logs.Info("Downloading "+config.Repourl+config.Versionfile+" to "+config.Tmpfolder+config.Versionfile)
	if err != nil {
		logs.Error("Error Copying downloaded file: "+err.Error())
	}
}

func DownloadFile(filepath string, url string)(err error){
	//Get the data	
    resp, err := http.Get(url)
    logs.Info("respuesta HTTP: --> ")
    logs.Info(resp)

    if err != nil {
		logs.Error("Error downloading file: "+err.Error())
        return err
    }
    defer resp.Body.Close()
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		logs.Error("Error creating file after download: "+err.Error())
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logs.Error("Error Copying downloaded file: "+err.Error())
		return err
	}
	return nil
}

func GetVersion(path string)(version string, err error){
	//check if exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "",err
	}
	f, err := os.Open(path)
	if err != nil {
		logs.Error("Error GetVersion OPEN: "+err.Error())
		return "",err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		logs.Error("Error GetVersion READ: "+err.Error())
		return "",err
	}
	return string(b),nil
}

func CheckVersion(confpath string) (status bool, err error){
	newVersion,err := GetVersion(config.Tmpfolder+config.Versionfile)
	if err != nil {	logs.Error("CheckVersion Error checking newVersion: "+err.Error()); return true,err}

	localVersion,err := GetVersion(confpath+config.Versionfile)
	if err != nil {	logs.Info("CheckVersion current.version doesn't exists: "+err.Error()); return true,err}

	newVersion = strings.TrimSuffix(newVersion, "\n")
	localVersion = strings.TrimSuffix(localVersion, "\n")
	
	splitNew := strings.Split(newVersion, ".")
	logs.Info(splitNew)

	splitLocal := strings.Split(localVersion, ".")
	logs.Info(splitLocal)

	for x := 0; x < 3 ; x++ {
		new,_ := strconv.Atoi(splitNew[x])  
		local,_ := strconv.Atoi(splitLocal[x])
		if new > local {
			logs.Info("New version available")
			return true, nil
		}
	}
	return false,nil
}

func RemoveDownloadedFiles(service string)(err error){
	err = os.RemoveAll(config.Tmpfolder+service)
	if err != nil {	logs.Error("RemoveDownloadedFiles Error Removing "+service+" path: "+err.Error()); return err }
	switch service {
		case "owlhmaster":
			err = os.RemoveAll(config.Tmpfolder+config.Mastertarfile)
			if err != nil {	logs.Error("RemoveDownloadedFiles Error Removing downloaded file for "+service+": "+err.Error()); return err }
			logs.Info("Files removed for "+service+" successfully!")
		case "owlhnode":
			err = os.RemoveAll(config.Tmpfolder+config.Nodetarfile)
			if err != nil {	logs.Error("RemoveDownloadedFiles Error Removing downloaded file for "+service+": "+err.Error()); return err }
			logs.Info("Files removed for "+service+" successfully!")
		case "owlhui":
			err = os.RemoveAll(config.Tmpfolder+config.Uitarfile)
			if err != nil {	logs.Error("RemoveDownloadedFiles Error Removing downloaded file for "+service+": "+err.Error()); return err }
			logs.Info("Files removed for "+service+" successfully!")
		default:
			logs.Info("UNKNOWN Service for RemoveDownloadedFiles. Files don't removed")
	}
	return nil
}

func RemoveCurrentVersion()(err error){
    err = os.RemoveAll(config.Tmpfolder+config.Versionfile)
	if err != nil {	logs.Error("RemoveDownloadedFiles Error Removing version file: "+err.Error()); return err }
    return nil
}


func FullCopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {	logs.Error("FullCopyFile Error opening file "+src+": "+err.Error()); return err }
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {	logs.Error("FullCopyFile Error creating destination "+dst+" file: "+err.Error()); return err }
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {	logs.Error("FullCopyFile Error Copying file in memory from "+src+" to "+dst+": "+err.Error()); return err }

	err = out.Sync()
	if err != nil {	logs.Error("FullCopyFile Error Sync source file from "+src+": "+err.Error()); return err }

	si, err := os.Stat(src)
	if err != nil {	logs.Error("FullCopyFile Error readding permission from "+src+": "+err.Error()); return err }

	err = os.Chmod(dst, si.Mode())
	if err != nil {	logs.Error("FullCopyFile Error setting permission to "+dst+": "+err.Error()); return err }

	return err
}

func FullCopyDir(src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {	logs.Error("Source directory doesn't exist: "+err.Error()); return err }
	if !si.IsDir() { logs.Error("Source is not a directory: "+err.Error()) ; return err}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {	logs.Error("MkDirAll error: "+err.Error()) }

	entries, err := ioutil.ReadDir(src)
	if err != nil {logs.Error("ReadDir source dir error: "+err.Error()); return err}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = FullCopyDir(srcPath, dstPath)
			if err != nil {	logs.Error("FullCopyDir error copy dir from "+srcPath+" to "+dstPath+": "+err.Error()) }
		} else {
			err = FullCopyFile(srcPath, dstPath)
			if err != nil {	logs.Error("FullCopyFile error copy file from "+srcPath+" to "+dstPath+": "+err.Error()) }
		}
	}

	return err
}

func systemType()(stype string){
	if _, err := os.Stat("/etc/systemd/system/"); !os.IsNotExist(err) {
		return "systemd"
	}else{
		return "systemV"
	}
}