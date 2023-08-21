package main

import (
	"dfs/client_handler"
	"dfs/helper"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var fileData = make(map[string][]byte) //chunkname, byte

func main() {
	if len(os.Args) < 2 {
		panic("Not enough arguments. Required arguments Action FilePath ChunkSize")
	}
	action := os.Args[1]
	host := "orion02:5000"
	fmt.Println("Connecting to ", host)
	conn, err := net.Dial("tcp", host)
	helper.CheckErr(err)
	clientHandler := client_handler.NewFileHandler(conn)
	switch {
	case action == "put":
		if len(os.Args) < 4 {
			panic("Not enough arguments. Required arguments <Action> <FilePath> <ChunkSize>")
		}
		//TODO: Default chunk size
		filePath := os.Args[2]
		filename := filepath.Base(filePath)
		chunksize, err := strconv.ParseInt(os.Args[3], 10, 64)
		helper.CheckErr(err)
		putFile(clientHandler, filePath, filename, chunksize)
	case action == "get":
		if len(os.Args) < 4 {
			panic("Not enough arguments. Required arguments <Action> <FileName> <OutputFilePath>")
		}
		filename := os.Args[2]
		outputFilePath := os.Args[3]
		getFile(clientHandler, outputFilePath, filename)
	case action == "delete":
		if len(os.Args) < 3 {
			panic("Not enough arguments. Required arguments <Action> <FileName>")
		}
		filename := os.Args[2]
		deleteFile(clientHandler, filename)
	case action == "ls":
		if len(os.Args) < 2 {
			panic("Not enough arguments. Required arguments <Action>")
		}
		listFiles(clientHandler)

	default:
		fmt.Println("Unexpected action. Please choose between get/put")
	}
}
func deleteFile(fileHandler *client_handler.FileHandler, filename string) {
	fileHandler.Send(&client_handler.ClientMsg{Filename: filename, Action: "delete"}) //filename not needed
	response, err := fileHandler.Receive()
	helper.CheckErr(err)
	replicaDetails := response.GetReplicaDetails()

	for chunkName, replicaNodesObj := range replicaDetails {

		if strings.Contains(chunkName, filename) {
			replicaNodesList := replicaNodesObj.GetReplicaList()
			// machineName := ""
			// replicaChunkName := ""
			for i := 0; i < len(replicaNodesList); i++ {
				// if i == 0 {
				// 	machineName = replicaNodesList[i]
				// 	replicaChunkName = chunkName
				// } else {
				// 	machineName = replicaNodesList[i]
				// 	replicaChunkName = chunkName + "_r"
				// }
				fmt.Println("machinename::", replicaNodesList[i])
				conn, err := net.Dial("tcp", replicaNodesList[i])
				if err != nil {
					fmt.Println("Connection with ", replicaNodesList[i], " failed.")
				}
				clientHandler := client_handler.NewFileHandler(conn)
				clientHandler.Send(&client_handler.ClientMsg{Filename: chunkName, Action: "delete"})
			}
			fmt.Println("Done deleteing files.")
		}
	}

}

func putFile(fileHandler *client_handler.FileHandler, filepath string, filename string, chunksize int64) {
	// checksum := helper.GetFileCheckSum(filepath)
	filesize := helper.GetFileSize(filepath)
	//Send metadata
	fileHandler.Send(&client_handler.ClientMsg{Filename: filename, Action: "put"}) //, Checksum: checksum, Filesize: filesize})
	response, err := fileHandler.Receive()
	helper.CheckErr(err)
	fmt.Println(response.GetStatus())
	if response.GetStatus() == "Ok" {
		data, err := os.ReadFile(filepath)
		helper.CheckErr(err)
		file := &client_handler.ClientMsg{Data: data, Filename: filename, Action: "put", Chunksize: chunksize, Filesize: filesize} //, Checksum: checksum
		fileHandler.Send(file)
	}

	//controller will send storage node location to store files
	response, err = fileHandler.Receive()
	chunkDetails := response.GetChunkDetails()
	replicaDetails := response.GetReplicaDetails()
	// fmt.Println("replicaList ::::: ", replicaDetails["file.txt_1"].GetReplicaList())
	helper.CheckErr(err)
	//disconnect with controller
	fileHandler.Close()
	// fmt.Println("Length Vhunk::",len(chunkDetails))

	if len(chunkDetails) > 0 {
		//divide file in chunks
		requiredChunks := int(filesize/(chunksize*1000000)) + 1 //filesize in bytes / chunksize in bytes => number of chunks in int
		bytesPerChunk := chunksize * 1000000
		// fileSourcePath := "/home/gakulkarni/cs_677/P1-semicolon/files/file.txt" //
		data, err := os.ReadFile(filepath)
		helper.CheckErr(err)
		// fmt.Println("DataLen::", len(data), " map:", chunkDetails["chunk_1"])
		index := int64(0)
		for i := 0; i < requiredChunks; i++ {
			//get bytes and send to storage nodes
			// buffer[i%index] = buffer[i%index] + data[i]
			key := filename + "_" + strconv.Itoa(i+1) //
			if i < requiredChunks-1 {
				// fmt.Println("if---", i, requiredChunks)
				chunkFileName := filename + "_" + strconv.Itoa(i+1) //filename_1
				sendToStorageNode(chunkDetails[key], data[index:index+bytesPerChunk], chunkFileName, replicaDetails)
				index = index + bytesPerChunk
			} else {
				// fmt.Println("else")
				chunkFileName := filename + "_" + strconv.Itoa(i+1)
				sendToStorageNode(chunkDetails[key], data[index:], chunkFileName, replicaDetails)
			}
		}
	} else {
		fmt.Println(response.GetStatus())
	}

}

func getFile(fileHandler *client_handler.FileHandler, outputFilePath string, filename string) {
	fmt.Println("In get File")

	//Send metadata
	fileHandler.Send(&client_handler.ClientMsg{Filename: filename, Action: "get"}) //, Checksum: checksum, Filesize: filesize})

	//Get File chunk location and replica location
	response, err := fileHandler.Receive()
	helper.CheckErr(err)

	if strings.Contains(response.GetStatus(), "SUCCESS") {
		chunkDetails := response.GetChunkDetails()
		replicaDetails := response.GetReplicaDetails()
		var waitg sync.WaitGroup
		//Iterate chunk details <k,v> chunk number, storage node name
		for chunkName, machineName := range chunkDetails {
			conn, err := net.Dial("tcp", machineName)
			if err == nil {
				//Get chunk from storage node //Original node is up
				clientHandler := client_handler.NewFileHandler(conn)
				waitg.Add(1)
				go getChunkFromStorageNode(clientHandler, chunkName, replicaDetails, &waitg)
			} else {
				//If original storage node is down, check replica for same chunk
				//Iterate replica details
				fmt.Println("Connection with storage node with original copy failed")
				replicaMachineList := replicaDetails[chunkName].GetReplicaList()
				allMachinesDown := true
				for i := 1; i < len(replicaMachineList); i++ {
					conn, err := net.Dial("tcp", replicaMachineList[i])
					if err == nil {
						fmt.Println("Getting ", chunkName, " from ", replicaMachineList[i])
						clientHandler := client_handler.NewFileHandler(conn)
						waitg.Add(1)
						go getChunkFromStorageNode(clientHandler, chunkName+"_r", replicaDetails, &waitg)
						allMachinesDown = false
						break
					}
				}
				if allMachinesDown {
					fmt.Println("All replicas are down for requested file", filename, ". Try again later, and hope for the best")
					panic("Goodbye")
				}
			}
		}
		waitg.Wait()

		file, err := os.OpenFile(outputFilePath+string(filepath.Separator)+filename, os.O_CREATE|os.O_WRONLY, 0644)
		helper.CheckErr(err)
		defer file.Close()
		for i := 0; i < len(fileData); i++ {
			chunkName := filename + "_" + strconv.Itoa(i+1)
			file.Write(fileData[chunkName])
		}
		fmt.Println("Received file.")
	} else {
		fmt.Println("File not found on the storage node.")
	}
}

func sendToStorageNode(machineName string, data []byte, filename string, replicaDetails map[string]*client_handler.ClientMsgReplicaNodes) {
	// fmt.Println("MachineName:",machineName)
	conn, err := net.Dial("tcp", machineName)
	helper.CheckErr(err)
	clientHandler := client_handler.NewFileHandler(conn)
	chunkChecksum := helper.GetChunkCheckSum(data)
	clientHandler.Send(&client_handler.ClientMsg{Data: data, Filename: filename, Checksum: chunkChecksum, Action: "put", ReplicaDetails: replicaDetails})
	// fileHandler.Send(&client_handler.ClientMsg{Filename: filename, Action: "put", Checksum: checksum, Filesize: filesize})
	fmt.Println("Sending to ", machineName, " storage node", filename)
	conn.Close()
}

func getChunkFromStorageNode(clientHandler *client_handler.FileHandler, chunkName string, replicaDetails map[string]*client_handler.ClientMsgReplicaNodes, waitg *sync.WaitGroup) {
	defer waitg.Done()
	//Request chunk from storage node
	clientHandler.Send(&client_handler.ClientMsg{Action: "get", Filename: chunkName, ReplicaDetails: replicaDetails})

	//Receive chunk from storage node
	response, err := clientHandler.Receive()
	helper.CheckErr(err)
	//cleaning chunk name to remove _r
	chunkName = strings.TrimSuffix(chunkName, "_r")
	fileData[chunkName] = response.GetData()
}

func listFiles(fileHandler *client_handler.FileHandler) {
	fmt.Println("In list files")
	fileHandler.Send(&client_handler.ClientMsg{Action: "ls"})

	response, err := fileHandler.Receive()
	helper.CheckErr(err)
	fmt.Println("FileList::", response.GetFileList())
	fmt.Println("DiskMapList::", response.GetDiskSpaceMap())
}
