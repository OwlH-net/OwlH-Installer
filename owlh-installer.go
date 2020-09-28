package main

import (
    "bufio"
    "bytes"
    "database/sql"
    "encoding/json"
    "errors"
    "github.com/astaxie/beego/logs"
    _ "github.com/mattn/go-sqlite3"
    "io/ioutil"
    "os"
    "os/exec"
    "regexp"
    "strings"
    "time"
)

type Config struct {
    Versionfile       string   `json:"versionfile"`
    Masterbinpath     string   `json:"masterbinpath"`
    Masterconfpath    string   `json:"masterconfpath"`
    Mastertarfile     string   `json:"mastertarfile"`
    Masterprescripts  string   `json:"masterprescripts"`
    Masterpostscripts string   `json:"masterpostscripts"`
    Nodebinpath       string   `json:"nodebinpath"`
    Nodeconfpath      string   `json:"nodeconfpath"`
    Nodetarfile       string   `json:"nodetarfile"`
    Nodeprescripts    string   `json:"nodeprescripts"`
    Nodepostscripts   string   `json:"nodepostscripts"`
    Uipath            string   `json:"uipath"`
    Uiconfpath        string   `json:"uiconfpath"`
    Uitarfile         string   `json:"uitarfile"`
    Uiprescripts      string   `json:"uiprescripts"`
    Uipostscripts     string   `json:"uipostscripts"`
    Tmpfolder         string   `json:"tmpfolder"`
    Target            []string `json:"target"`
    Uifiles           []string `json:"uifiles"`
    Action            string   `json:"action"`
    Repourl           string   `json:"repourl"`
    Masterfiles       []string `json:"masterfiles"`
    Nodefiles         []string `json:"nodefiles"`
    Masterdb          []string `json:"masterdb"`
    Nodedb            []string `json:"nodedb"`
}

var file = "log.json"
var f *os.File
var config Config

func ReadConfig() Config {
    configStruct, err := os.Open("config.json")
    if err != nil {
        logs.Error(err)
    }

    defer configStruct.Close()
    b, err := ioutil.ReadAll(configStruct)
    if err != nil {
        logs.Error(err)
    }

    var localConfig Config
    json.Unmarshal([]byte(b), &localConfig)

    return localConfig
}

func RunShScript(shpath string, action string) (err error) {
    if !fileExists(shpath) {
        return errors.New("File " + shpath + " does not exist")
    }
    outRemote, err := exec.Command("bash", shpath, action).Output()
    if err != nil {
        logs.Error("RunShScript: " + shpath + " -> " + err.Error())
        return err
    }
    logs.Info(string(outRemote))
    return nil
}

func UpdateJsonFile(newFile string, currentFile string) {
    local, err := os.Open(currentFile)
    if err != nil {
        logs.Error(err)
    }

    remote, err := os.Open(newFile)
    if err != nil {
        logs.Error(err)
    }

    defer local.Close()
    defer remote.Close()

    b, err := ioutil.ReadAll(local)
    if err != nil {
        logs.Error(err)
    }

    c, err := ioutil.ReadAll(remote)
    if err != nil {
        logs.Error(err)
    }

    var localFile map[string]interface{}
    var remoteFile map[string]interface{}
    json.Unmarshal([]byte(b), &localFile)
    json.Unmarshal([]byte(c), &remoteFile)

    CompareJSONFile(localFile, remoteFile)

    LinesOutput, _ := json.Marshal(localFile)
    var out bytes.Buffer
    json.Indent(&out, LinesOutput, "", "\t")
    ioutil.WriteFile(currentFile, out.Bytes(), 0644)

    return
}

func UpdateDBFile(currentDB string, newDB string) {
    outRemote, err := exec.Command("sqlite3", newDB, ".table").Output()
    if err != nil {
        logs.Error("UpdateDBFile outRemote: " + err.Error())
    }
    outLocal, err := exec.Command("sqlite3", currentDB, ".table").Output()
    if err != nil {
        logs.Error("UpdateDBFile outLocal: " + err.Error())
    }

    re := regexp.MustCompile(`\s+`)
    outputRemote := re.ReplaceAllString(string(outRemote), "\n")
    outputLocal := re.ReplaceAllString(string(outLocal), "\n")
    splitLocal := strings.Split(outputLocal, "\n")
    splitRemote := strings.Split(outputRemote, "\n")

    var exists bool
    for w := range splitRemote {
        exists = false
        for z := range splitLocal {
            if splitRemote[w] == splitLocal[z] {
                exists = true
            }
        }
        if !exists {
            createTable, err := exec.Command("sqlite3", newDB, ".schema "+splitRemote[w]).Output()
            if err != nil {
                logs.Error("UpdateDBFile Error Create table: " + err.Error())
            }
            database, err := sql.Open("sqlite3", currentDB)
            if err != nil {
                logs.Error("UpdateDBFile Error Open table: " + err.Error())
            }
            statement, err := database.Prepare(string(createTable))
            if err != nil {
                logs.Error("UpdateDBFile Error Prepare table: " + err.Error())
            }
            defer database.Close()
            statement.Exec()
        }
    }
}

func Logger(data map[string]string) {
    f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        logs.Error(err)
        return
    }
    defer f.Close()

    LinesOutput, _ := json.Marshal(data)

    _, err = f.WriteString(string(LinesOutput) + "\n")
    if err != nil {
        logs.Error(err)
        return
    }

    return
}

func GetNewSoftware(service string) (err error) {
    var url string
    var tarfile string
    switch service {
    case "owlhmaster":
        tarfile = config.Mastertarfile
        url = config.Repourl + tarfile
    case "owlhnode":
        tarfile = config.Nodetarfile
        url = config.Repourl + tarfile
    case "owlhui":
        tarfile = config.Uitarfile
        url = config.Repourl + tarfile
    default:
        return errors.New("UNKNOWN service to download GetNewSoftware")
    }

    err = DownloadFile(config.Tmpfolder+tarfile, url)
    if err != nil {
        logs.Error("Error GetNewSoftware downloading: " + err.Error())
        return err
    }
    err = ExtractTarGz(config.Tmpfolder+tarfile, config.Tmpfolder+service)
    if err != nil {
        logs.Error("Error GetNewSoftware extracting: " + err.Error())
        return err
    }

    return nil
}

func CopyBinary(service string) (err error) {
    binFileSrc := config.Tmpfolder + service + "/" + service
    var binFileDst string
    switch service {
    case "owlhmaster":
        binFileDst = config.Masterbinpath + service
        err = os.MkdirAll(config.Masterbinpath, 0755)
        if err != nil {
            logs.Error("CopyBinary MkDirAll error creating folder for Master: " + err.Error())
        }
    case "owlhnode":
        binFileDst = config.Nodebinpath + service
        err = os.MkdirAll(config.Nodebinpath, 0755)
        if err != nil {
            logs.Error("CopyBinary MkDirAll error creating folder for Node: " + err.Error())
        }
    default:
        return errors.New("UNKNOWN service to download CopyBinary")
    }

    err = CopyFiles(binFileSrc, binFileDst)
    if err != nil {
        logs.Error("CopyBinary Error copy files: " + err.Error())
        return err
    }

    return err
}

func UpdateTxtFile(src string, dst string) (err error) {
    local, err := os.Open(src)
    if err != nil {
        logs.Error(err)
        return err
    }

    remote, err := os.Open(dst)
    if err != nil {
        logs.Error("Error opennign file for read UpdateTxtFile: " + err.Error())
        return err
    }
    remoteWR, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil {
        logs.Error("Error opennign file for append UpdateTxtFile: " + err.Error())
        return err
    }

    defer local.Close()
    defer remote.Close()
    defer remoteWR.Close()

    scannerSRC := bufio.NewScanner(local)
    scannerDST := bufio.NewScanner(remote)

    var totalLine []string
    dstLine := make(map[string]string)

    for scannerDST.Scan() {
        dLine := strings.Split(scannerDST.Text(), " ")
        dstLine[dLine[0]] = scannerDST.Text()
    }

    for scannerSRC.Scan() {
        srcLine := strings.Split(scannerSRC.Text(), " ")
        if _, ok := dstLine[srcLine[0]]; !ok {
            totalLine = append(totalLine, scannerSRC.Text())
        }
    }

    for x := range totalLine {
        if _, err = remoteWR.WriteString("\n" + totalLine[x]); err != nil {
            logs.Error("Error writting to dst file: " + err.Error())
            return err
        }
    }
    return err
}

func UpdateFiles(service string) (err error) {
    switch service {
    case "owlhmaster":
        for w := range config.Masterfiles {
            if _, err := os.Stat(config.Masterconfpath + config.Masterfiles[w]); os.IsNotExist(err) {
                err = CopyFiles(config.Tmpfolder+service+"/conf/"+config.Masterfiles[w], config.Masterconfpath+config.Masterfiles[w])
                if err != nil {
                    logs.Error("UpdateFiles Error copy files for master: " + err.Error())
                    return err
                }
            } else {
                if config.Masterfiles[w] == "app.conf" {
                    err = UpdateTxtFile(config.Tmpfolder+service+"/conf/"+config.Masterfiles[w], config.Masterconfpath+config.Masterfiles[w])
                    if err != nil {
                        logs.Error("UpdateTxtFile Error copy files for master: " + err.Error())
                        return err
                    }
                } else {
                    UpdateJsonFile(config.Tmpfolder+service+"/conf/"+config.Masterfiles[w], config.Masterconfpath+config.Masterfiles[w])
                }
            }
        }
        err = CopyFiles(config.Tmpfolder+"current.version", config.Masterconfpath+"current.version")
        if err != nil {
            logs.Error("UpdateFiles Error CopyFiles for assign current current.version file: " + err.Error())
            return err
        }

    case "owlhnode":
        for w := range config.Nodefiles {
            if _, err := os.Stat(config.Nodeconfpath + config.Nodefiles[w]); os.IsNotExist(err) {
                err = CopyFiles(config.Tmpfolder+service+"/conf/"+config.Nodefiles[w], config.Nodeconfpath+config.Nodefiles[w])
                if err != nil {
                    logs.Error("UpdateFiles Error copy files for Node: " + err.Error())
                    return err
                }
            } else {
                if config.Nodefiles[w] == "app.conf" {
                    err = UpdateTxtFile(config.Tmpfolder+service+"/conf/"+config.Nodefiles[w], config.Nodeconfpath+config.Nodefiles[w])
                    if err != nil {
                        logs.Error("UpdateTxtFile Error copy files for Node: " + err.Error())
                        return err
                    }
                } else {
                    UpdateJsonFile(config.Tmpfolder+service+"/conf/"+config.Nodefiles[w], config.Nodeconfpath+config.Nodefiles[w])
                }
            }
        }
    default:
        return errors.New("UNKNOWN service to download UpdateFiles")
    }

    return nil
}

func UpdateDb(service string) (err error) {
    switch service {
    case "owlhmaster":
        for w := range config.Masterdb {
            if _, err := os.Stat(config.Masterconfpath + config.Masterdb[w]); os.IsNotExist(err) {
                err = CopyFiles(config.Tmpfolder+service+"/conf/"+config.Masterdb[w], config.Masterconfpath+config.Masterdb[w])
                if err != nil {
                    logs.Error("UpdateDb Error copy files for master: " + err.Error())
                    return err
                }
            } else if strings.Contains(config.Masterdb[w], ".db") {
                UpdateDBFile(config.Masterconfpath+config.Masterdb[w], config.Tmpfolder+service+"/conf/"+config.Masterdb[w])
            }
        }
    case "owlhnode":
        for w := range config.Nodedb {
            if _, err := os.Stat(config.Nodeconfpath + config.Nodedb[w]); os.IsNotExist(err) {
                err = CopyFiles(config.Tmpfolder+service+"/conf/"+config.Nodedb[w], config.Nodeconfpath+config.Nodedb[w])
                if err != nil {
                    logs.Error("UpdateDb Error copy files for node: " + err.Error())
                    return err
                }
            } else if strings.Contains(config.Nodedb[w], ".db") {
                UpdateDBFile(config.Nodeconfpath+config.Nodedb[w], config.Tmpfolder+service+"/conf/"+config.Nodedb[w])
            }
        }
    default:
        return errors.New("UNKNOWN service to download UpdateDb")
    }

    return nil
}

func StartService(service string) (err error) {
    systemCtl := "systemctl"
    restart := "restart"
    if service == "owlhui" {
        if _, err := os.Stat("/etc/systemd/system/"); !os.IsNotExist(err) {
            logs.Info(service + " OwlH UI - systemd starting...")
            _, err := exec.Command("bash", "-c", "systemctl restart httpd").Output()
            return err
        } else if _, err := os.Stat("/etc/init.d/" + service); !os.IsNotExist(err) {
            logs.Info(service + " OwlH UI - systemV starting...")
            _, err := exec.Command("bash", "-c", "service httpd restart").Output()
            return err
        }
    }
    if _, err := os.Stat("/etc/systemd/system/" + service + ".service"); !os.IsNotExist(err) {
        logs.Info(service + " systemd starting...")
        _, err := exec.Command(systemCtl, restart, service).Output()
        return err
    } else if _, err := os.Stat("/etc/init.d/" + service); !os.IsNotExist(err) {
        logs.Info(service + " systemV starting...")
        _, err := exec.Command("bash", "-c", "service "+service+" start").Output()
        return err
    }
    logs.Info(service + " -> no service installed (systemd or sysV file not found)")
    logs.Info("I can't start the service...")
    return nil
}

func StopService(service string) error {
    systemCtl := "systemctl"
    stop := "stop"
    if _, err := os.Stat("/etc/systemd/system/" + service + ".service"); !os.IsNotExist(err) {
        logs.Info(service + " systemd stopping...")
        _, err := exec.Command(systemCtl, stop, service).Output()
        return err
    } else if _, err := os.Stat("/etc/init.d/" + service); !os.IsNotExist(err) {
        logs.Info(service + " systemV stopping...")
        _, err := exec.Command("bash", "-c", "service "+service+" stop").Output()
        return err
    }

    logs.Info(service + " -> can't find service script, killing any previous running instance")
    exec.Command("bash", "-c", "kill -9 $(pidof "+service+")").Output()
    return nil
}

func BackupUiConf() (err error) {
    for x := range config.Uifiles {
        err = CopyFiles(config.Uiconfpath+config.Uifiles[x], config.Tmpfolder+config.Uifiles[x]+".bck")
        if err != nil {
            logs.Error("BackupUiConf Error CopyFiles for make a backup: " + err.Error())
            return err
        }
    }
    return nil
}

func RestoreBackups() (err error) {
    for x := range config.Uifiles {
        err = CopyFiles(config.Tmpfolder+config.Uifiles[x]+".bck", config.Uiconfpath+config.Uifiles[x])
        if err != nil {
            logs.Error("BackupUiConf Error CopyFiles for make a backup: " + err.Error())
            return err
        }
        err = os.RemoveAll(config.Tmpfolder + config.Uifiles[x] + ".bck")
        if err != nil {
            logs.Error("RemoveDownloadedFiles Error Removing version file: " + err.Error())
            return err
        }
    }

    return nil
}

func CopyServiceFiles(service string) (err error) {

    systemCtl := "systemctl"
    daemonReload := "daemon-reload"
    enable := "enable"
    switch service {
    case "owlhmaster":
        src := config.Masterbinpath + "conf/service/owlhmaster.service"
        dst := "/etc/systemd/system/owlhmaster.service"
        err = FullCopyFile(src, dst)
        if err != nil {
            logs.Warning("CopyServiceFiles systemd ERROR: " + err.Error())
            return err
        }
    case "owlhnode":
        src := config.Nodebinpath + "conf/service/owlhnode.service"
        dst := "/etc/systemd/system/owlhnode.service"
        err = FullCopyFile(src, dst)
        if err != nil {
            logs.Warning("CopyServiceFiles systemd ERROR: " + err.Error())
            return err
        }
    default:
        logs.Warning("No service or UNKNOWN %s", service)
        return nil
    }
    _, err = exec.Command(systemCtl, daemonReload).Output()
    if err != nil {
        logs.Info("Reload Daemon configuration -> %s", err.Error())
    }
    _, err = exec.Command(systemCtl, enable, service).Output()
    if err != nil {
        logs.Info("Enable service %s -> %s", service, err.Error())
    }

    return nil
}

func FindFolderScripts(folder string, action string) (err error) {
    if _, err := os.Stat(folder); !os.IsNotExist(err) {
        logs.Info("Find script files on path -> " + folder)
        files := getFilesFromFolder(folder)
        for file := range files {
            logs.Info("Script found -> " + files[file])
            RunShScript(files[file], action)
        }
    }
    return nil
}

func RunPreScripts(service string, action string) {

    switch service {
    case "owlhnode":
        logs.Info("PRESCRIPTS - NODE -> " + config.Nodeprescripts)
        if _, err := os.Stat(config.Nodeprescripts); !os.IsNotExist(err) {
            logs.Info("PRESCRIPTS - NODE -> Let's run -> ")
            FindFolderScripts(config.Nodeprescripts, action)
        }
    case "owlhmaster":
        logs.Info("PRESCRIPTS - MASTER -> " + config.Masterprescripts)
        if _, err := os.Stat(config.Masterprescripts); !os.IsNotExist(err) {
            logs.Info("PRESCRIPTS - MASTER -> Let's run -> ")
            FindFolderScripts(config.Masterprescripts, action)
        }
    case "owlhui":
        logs.Info("PRESCRIPTS - UI -> " + config.Uiprescripts)
        if _, err := os.Stat(config.Uiprescripts); !os.IsNotExist(err) {
            logs.Info("PRESCRIPTS - UI -> Let's run -> ")
            FindFolderScripts(config.Uiprescripts, action)
        }
    default:
        logs.Info("PRESCRIPTS - Not a Service -> " + service)
    }
    return
}

func RunPostScripts(service string, action string) {

    switch service {
    case "owlhnode":
        logs.Info("POSTSCRIPTS - NODE -> " + config.Nodepostscripts)
        if _, err := os.Stat(config.Nodepostscripts); !os.IsNotExist(err) {
            logs.Info("POSTSCRIPTS - NODE -> Let's run -> ")
            FindFolderScripts(config.Nodepostscripts, action)
        }
    case "owlhmaster":
        logs.Info("POSTSCRIPTS - MASTER -> " + config.Masterpostscripts)
        if _, err := os.Stat(config.Masterpostscripts); !os.IsNotExist(err) {
            logs.Info("POSTSCRIPTS - MASTER -> Let's run -> ")
            FindFolderScripts(config.Masterpostscripts, action)
        }
    case "owlhui":
        logs.Info("POSTSCRIPTS - UI -> " + config.Uipostscripts)
        if _, err := os.Stat(config.Uipostscripts); !os.IsNotExist(err) {
            logs.Info("POSTSCRIPTS - UI -> Let's run -> ")
            FindFolderScripts(config.Uipostscripts, action)
        }
    default:
        logs.Info("POSTSCRIPTS - Not a Service -> " + service)
    }
    return
}

func ManageMaster() {
    var err error
    isError := false
    sessionLog := make(map[string]string)
    currentTime := time.Now().Format("2006-01-02 15:04:05")
    sessionLog["date"] = currentTime
    service := "owlhmaster"
    logs.Info("== MASTER ==")
    sessionLog["status"] = "== MASTER =="
    Logger(sessionLog)

    RunPreScripts(service, config.Action)

    switch config.Action {
    case "install":
        logs.Info("Master INSTALL")
        sessionLog["status"] = "New Install for Master"
        Logger(sessionLog)

        logs.Info("Downloading New Software")
        err = GetNewSoftware(service)
        if err != nil {
            logs.Error("ManageMaster Error INSTALL GetNewSoftware: " + err.Error())
            sessionLog["status"] = "Error getting new software for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
            return
        }

        logs.Info("ManageMaster Stopping the service")
        err = StopService(service)
        if err != nil {
            logs.Warning("ManageMaster Error INSTALL StopService: " + err.Error())
            sessionLog["status"] = "Error stopping service for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        logs.Info("ManageMaster Copying files from download")
        err = CopyBinary(service)
        if err != nil {
            logs.Error("ManageMaster Error INSTALL CopyBinary: " + err.Error())
            sessionLog["status"] = "Error copying binary for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        err = FullCopyDir(config.Tmpfolder+service+"/conf/", config.Masterconfpath)
        if err != nil {
            logs.Error("FullCopyDir Error INSTALL Master: " + err.Error())
            sessionLog["status"] = "Error copying full directory for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        err = FullCopyDir(config.Tmpfolder+service+"/defaults/", config.Masterconfpath+"defaults/")
        if err != nil {
            logs.Error("FullCopyDir Error INSTALL Master: " + err.Error())
            sessionLog["status"] = "Error copying full directory for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        logs.Info("ManageMaster Installing service...")
        err = CopyServiceFiles(service)
        if err != nil {
            logs.Warning("CopyServiceFiles Error INSTALL Master: " + err.Error())
            sessionLog["status"] = "Error copying service files for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        logs.Info("ManageMaster Copying current.version...")
        err = CopyFiles(config.Tmpfolder+"current.version", config.Masterconfpath+"current.version")
        if err != nil {
            logs.Error("ManageMaster back up Error CopyFiles for assign current current.version file: " + err.Error())
            sessionLog["status"] = "Error copying files for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        logs.Info("ManageMaster Launching service...")
        err = StartService(service)
        if err != nil {
            logs.Warning("ManageMaster Error INSTALL StartService: " + err.Error())
            sessionLog["status"] = "Error launching service for Master: " + err.Error()
            Logger(sessionLog)
            isError = true
        }

        logs.Info("ManageMaster Done!")
        if isError {
            sessionLog["status"] = "ManageMaster installed with errors/warnings..."
        } else {
            sessionLog["status"] = "ManageMaster installed done!"
        }
        Logger(sessionLog)

    case "update":
        sessionLog["status"] = "Master UPDATE"
        Logger(sessionLog)
        logs.Info("Master UPDATE")

        needsUpdate, _ := CheckVersion(config.Masterconfpath)
        // if err != nil {  logs.Error("ManageMaster Error UPDATING needsUpdate: "+err.Error()); sessionLog["status"] = "Error checking version for Master: "+err.Error(); Logger(sessionLog); isError=true}
        if needsUpdate {

            err = GetNewSoftware(service)
            if err != nil {
                logs.Error("ManageMaster Error UPDATING GetNewSoftware: " + err.Error())
                sessionLog["status"] = "Error getting new software for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
                return
            }
            err = StopService(service)
            if err != nil {
                logs.Warning("ManageMaster Error UPDATING StopService: " + err.Error())
                sessionLog["status"] = "Error stopping service for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = CopyBinary(service)
            if err != nil {
                logs.Error("ManageMaster Error UPDATING CopyBinary: " + err.Error())
                sessionLog["status"] = "Error copying binary for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = UpdateFiles(service)
            if err != nil {
                logs.Error("ManageMaster Error UPDATING UpdateFiles: " + err.Error())
                sessionLog["status"] = "Error updating files for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = UpdateDb(service)
            if err != nil {
                logs.Error("ManageMaster Error UPDATING UpdateDb: " + err.Error())
                sessionLog["status"] = "Error updating DB for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = CopyFiles(config.Tmpfolder+"current.version", config.Masterconfpath+"current.version")
            if err != nil {
                logs.Error("ManageMaster Error CopyFiles for assign current current.version file: " + err.Error())
                sessionLog["status"] = "Error copying files for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = StartService(service)
            if err != nil {
                logs.Warning("ManageMaster Error UPDATING StartService: " + err.Error())
                sessionLog["status"] = "Error starting service for Master: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
        } else {
            logs.Info("Master is Up to date")
            if isError {
                sessionLog["status"] = "ManageMaster updated with errors..."
            } else {
                sessionLog["status"] = "ManageMaster updated done!"
            }
            Logger(sessionLog)
        }
    default:
        logs.Info("UNKNOWN Action ManageMaster")
        sessionLog["status"] = "UNKNOWN Action ManageMaster"
        Logger(sessionLog)
    }

    RunPostScripts(service, config.Action)
}
func ManageNode() {
    isError := false
    currentTime := time.Now().Format("2006-01-02 15:04:05")
    sessionLog := make(map[string]string)
    sessionLog["date"] = currentTime
    var err error
    service := "owlhnode"
    logs.Info("== NODE ==")
    sessionLog["status"] = "== NODE =="
    Logger(sessionLog)

    RunPreScripts(service, config.Action)

    switch config.Action {
    case "install":
        logs.Info("Node INSTALL")
        sessionLog["status"] = "Node INSTALL"
        Logger(sessionLog)

        logs.Info("Downloading New Software")
        err = GetNewSoftware(service)
        if err != nil {
            logs.Error("INSTALL ManageNode Error GetNewSoftware: " + err.Error())
            sessionLog["status"] = "Error getting new software for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
            return
        }
        logs.Info("ManageNode Stopping the service")
        err = StopService(service)
        if err != nil {
            logs.Warning("INSTALL ManageNode Error StopService: " + err.Error())
            sessionLog["status"] = "Error Stopping service for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        logs.Info("ManageNode Copying files from download")
        err = CopyBinary(service)
        if err != nil {
            logs.Error("INSTALL ManageNode Error  CopyBinary: " + err.Error())
            sessionLog["status"] = "Error copying binary for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        err = FullCopyDir(config.Tmpfolder+service+"/conf/", config.Nodeconfpath)
        if err != nil {
            logs.Error("INSTALL FullCopyDir Error Node: " + err.Error())
            sessionLog["status"] = "Error copying full conf directory for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        err = FullCopyDir(config.Tmpfolder+service+"/defaults/", config.Nodeconfpath+"defaults/")
        if err != nil {
            logs.Error("INSTALL FullCopyDir Error  Node: " + err.Error())
            sessionLog["status"] = "Error copying full defaults directory for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        err = CopyFiles(config.Tmpfolder+"current.version", config.Nodeconfpath+"current.version")
        if err != nil {
            logs.Error("INSTALL ManageNode Error CopyFiles for assign current current.version file: " + err.Error())
            sessionLog["status"] = "Error Copying files for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        logs.Info("ManageNode Installing service...")
        err = CopyServiceFiles(service)
        if err != nil {
            logs.Error("CopyServiceFiles Error INSTALL Node: " + err.Error())
            sessionLog["status"] = "Error copying service files for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        logs.Info("ManageNode Launching service...")
        err = StartService(service)
        if err != nil {
            logs.Warning("INSTALL ManageNode Error StartService: " + err.Error())
            sessionLog["status"] = "Error launching service for Node: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        logs.Info("ManageNode Done!")
        if isError {
            sessionLog["status"] = "ManageNode installed with errors..."
        } else {
            sessionLog["status"] = "ManageNode installed done!"
        }
        Logger(sessionLog)
    case "update":
        logs.Info("Node UPDATE")
        sessionLog["status"] = "Update for Node"
        Logger(sessionLog)
        needsUpdate, _ := CheckVersion(config.Nodeconfpath)
        // if err != nil {  logs.Error("ManageNode Error UPDATING needsUpdate: "+err.Error()); sessionLog["status"] = "Error Checking version for Node: "+err.Error(); Logger(sessionLog); isError=true}
        if needsUpdate {

            err = GetNewSoftware(service)
            if err != nil {
                logs.Error("ManageNode Error UPDATING GetNewSoftware: " + err.Error())
                sessionLog["status"] = "Error Getting new software for Node: " + err.Error()
                Logger(sessionLog)
                isError = true
                return
            }
            err = StopService(service)
            if err != nil {
                logs.Warning("ManageNode Error UPDATING StopService: " + err.Error())
                sessionLog["status"] = "Error Stopping service for Node:" + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = CopyBinary(service)
            if err != nil {
                logs.Error("ManageNode Error UPDATING CopyBinary: " + err.Error())
                sessionLog["status"] = "Error Copying Binary for Node: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = UpdateFiles(service)
            if err != nil {
                logs.Error("ManageNode Error UPDATING UpdateFiles: " + err.Error())
                sessionLog["status"] = "Error updating files for Node: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = UpdateDb(service)
            if err != nil {
                logs.Error("ManageNode Error UPDATING UpdateDb: " + err.Error())
                sessionLog["status"] = "Error updating DB for Node: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = CopyFiles(config.Tmpfolder+"current.version", config.Nodeconfpath+"current.version")
            if err != nil {
                logs.Error("ManageNode BackupUiConf Error CopyFiles for assign current current.version file: " + err.Error())
                sessionLog["status"] = "Error Copying files for Node: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = StartService(service)
            if err != nil {
                logs.Warning("ManageNode Error UPDATING StartService: " + err.Error())
                sessionLog["status"] = "Error launching service error for Node:" + err.Error()
                Logger(sessionLog)
                isError = true
            }
            if isError {
                sessionLog["status"] = "ManageNode updated with errors..."
            } else {
                sessionLog["status"] = "ManageNode updated done!"
            }
            Logger(sessionLog)
        } else {
            logs.Info("Node is Up to date")
            sessionLog["status"] = "Node is up to date"
            Logger(sessionLog)
        }
    default:
        logs.Info("UNKNOWN Action ManageNode")
        sessionLog["status"] = "UNKNOWN Action ManageNode"
        Logger(sessionLog)
    }

    RunPostScripts(service, config.Action)

}
func ManageUI() {
    isError := false
    currentTime := time.Now().Format("2006-01-02 15:04:05")
    sessionLog := make(map[string]string)
    sessionLog["date"] = currentTime

    var err error
    service := "owlhui"
    logs.Info("== UI ==")
    sessionLog["status"] = "== UI =="
    Logger(sessionLog)

    RunPreScripts(service, config.Action)

    switch config.Action {
    case "install":
        logs.Info("New Install for UI")
        sessionLog["status"] = "New Install for UI"
        Logger(sessionLog)

        logs.Info("Downloading New Software")
        err = GetNewSoftware(service)
        if err != nil {
            logs.Error("INSTALL ManageUI Error GetNewSoftware: " + err.Error())
            sessionLog["status"] = "Error getting new software for UI: " + err.Error()
            Logger(sessionLog)
            isError = true
            return
        }
        logs.Info("ManageUI Copying files from download")
        err = FullCopyDir(config.Tmpfolder+service, config.Uipath)
        if err != nil {
            logs.Error("INSTALL ManageUI Error FullCopyDir UI: " + err.Error())
            sessionLog["status"] = "Error copying full directory for UI: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        logs.Info("ManageUI Launching service...")
        err = CopyFiles(config.Tmpfolder+"current.version", config.Uiconfpath+"current.version")
        if err != nil {
            logs.Error("INSTALL ManageUI Error BackupUiConf CopyFiles for assign current current.version file: " + err.Error())
            sessionLog["status"] = "Error copying files for UI: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        err = StartService(service)
        if err != nil {
            logs.Error("INSTALL ManageUI Error StartService: " + err.Error())
            sessionLog["status"] = "Error starting service for UI: " + err.Error()
            Logger(sessionLog)
            isError = true
        }
        if isError {
            sessionLog["status"] = "ManageUI installed with errors..."
        } else {
            sessionLog["status"] = "ManageUI installation done!"
        }
        Logger(sessionLog)
        logs.Info("ManageUI Done!")
    case "update":
        sessionLog["status"] = "UI UPDATE"
        Logger(sessionLog)
        logs.Info("UI UPDATE")

        needsUpdate, _ := CheckVersion(config.Uiconfpath)
        if needsUpdate {

            err = GetNewSoftware(service)
            if err != nil {
                logs.Error("UPDATE ManageUI Error GetNewSoftware: " + err.Error())
                sessionLog["status"] = "Error Getting new software for UI: " + err.Error()
                Logger(sessionLog)
                isError = true
                return
            }
            err = BackupUiConf()
            if err != nil {
                logs.Error("UPDATE ManageUI Error ui.conf backup: " + err.Error())
                sessionLog["status"] = "Error backing up configuration file software for UI: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = FullCopyDir(config.Tmpfolder+service, config.Uipath)
            if err != nil {
                logs.Error("UPDATE ManageUI Error CopyAllUiFiles copying new elements to directory: " + err.Error())
                sessionLog["status"] = "Error copying full directory for UI: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = CopyFiles(config.Tmpfolder+"current.version", config.Uiconfpath+"current.version")
            if err != nil {
                logs.Error("UPDATE ManageUI Error BackupUiConf CopyFiles for assign current current.version file: " + err.Error())
                sessionLog["status"] = "Error copying files for UI: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            err = RestoreBackups()
            if err != nil {
                logs.Error("UPDATE ManageUI Error RestoreBackups: " + err.Error())
                sessionLog["status"] = "Error restoring backups for UI: " + err.Error()
                Logger(sessionLog)
                isError = true
            }
            if isError {
                sessionLog["status"] = "ManageUI updated with errors..."
            } else {
                sessionLog["status"] = "ManageUI updated done!"
            }
            Logger(sessionLog)
        } else {
            logs.Info("UI is Up to date")
            sessionLog["status"] = "UI is up to date"
            Logger(sessionLog)
        }
    default:
        logs.Info("UNKNOWN Action ManageUI")
        sessionLog["status"] = "UNKNOWN Action ManageUI"
        Logger(sessionLog)
    }
    RunPostScripts(service, config.Action)
}

func main() {

    version := "0.16.0.20200902"
    logs.Info("OwlH Installer - v%s", version)
    var err error
    currentTime := time.Now().Format("2006-01-02 15:04:05")
    sessionLog := make(map[string]string)
    sessionLog["date"] = currentTime
    sessionLog["status"] = "--- Start Installer/Updater ---"
    Logger(sessionLog)

    //Read Struct
    config = ReadConfig()
    //Download current version
    DownloadCurrentVersion()

    for w := range config.Target {
        switch config.Target[w] {
        case "owlhmaster":
            if _, err = os.Stat(config.Masterbinpath); !os.IsNotExist(err) {
                ManageMaster()
            } else if config.Action == "install" {
                ManageMaster()
            }
            err = RemoveDownloadedFiles(config.Target[w])
            if err != nil {
                logs.Error("Error removing " + config.Target[w] + " files: " + err.Error())
                sessionLog["status"] = "Error removing " + config.Target[w] + " files: " + err.Error()
                Logger(sessionLog)
            }
        case "owlhnode":
            if _, err = os.Stat(config.Nodebinpath); !os.IsNotExist(err) {
                ManageNode()
            } else if config.Action == "install" {
                ManageNode()
            }
            err = RemoveDownloadedFiles(config.Target[w])
            if err != nil {
                logs.Error("Error removing " + config.Target[w] + " files: " + err.Error())
                sessionLog["status"] = "Error removing " + config.Target[w] + " files: " + err.Error()
                Logger(sessionLog)
            }
        case "owlhui":
            if _, err = os.Stat(config.Uipath); !os.IsNotExist(err) {
                ManageUI()
            } else if config.Action == "install" {
                ManageUI()
            }
            err = RemoveDownloadedFiles(config.Target[w])
            if err != nil {
                logs.Error("Error removing " + config.Target[w] + " files: " + err.Error())
                sessionLog["status"] = "Error removing " + config.Target[w] + " files: " + err.Error()
                Logger(sessionLog)
            }
        default:
            logs.Info("UNKNOWN Target at Main()")
            sessionLog["status"] = "UNKNOWN Target at Main()"
            Logger(sessionLog)
        }
    }

    RemoveCurrentVersion()
    currentTime = time.Now().Format("2006-01-02 15:04:05")
    sessionLog = make(map[string]string)
    sessionLog["date"] = currentTime
    sessionLog["status"] = "--- End Updater ---"
    Logger(sessionLog)

    return

}
