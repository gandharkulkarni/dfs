//controller code
package main

import (
	"dfs/client_handler"
	"dfs/helper"
	"dfs/storageNode_handler"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"

	// "math/rand"
	"net"
	"os"
	"sync"
	"time"
)

var serverName_time = make(map[string]map[string]time.Time)
var fileStorageNodeMap = make(map[string]map[string]string)                        /*Map{FileName {chunk name, storage node name with port number seperated by colon}}*/
var replicaStorageNodeMap = make(map[string]*client_handler.ClientMsgReplicaNodes) //{chunkName (file.txt_1):{ ["orion05","orion06","orion07"]} } (orion05 original SN)(orion06 and orion07 duplicate SN)
var fileLocationMap = make(map[string][]string)                                    /*Map {Key: Orion03, Value: [file1, file2, file3]}*/
var diskSpaceMap = make(map[string]string)

func performOperations(storageNode *storageNode_handler.StorageNode) {
	//receive data
	dataReceived, _ := storageNode.Receive()
	//fmt.Println(dataReceived)
	machineName := dataReceived.GetMachineName()
	port := dataReceived.GetPort()
	currentTime := time.Now()
	diskSpace := dataReceived.GetDiskSpace()

	_, prs := serverName_time[machineName][port] //prs= false then no machine in map else true
	if !prs {
		//add to map
		// fmt.Println("New client connected:", machineName, ":", port)

		_, prst := serverName_time[machineName]
		if !prst {
			serverName_time[machineName] = map[string]time.Time{}
		}
		serverName_time[machineName][port] = currentTime
		response := &storageNode_handler.Response{
			Message: "SUCCESS: HeartBeat accepted. Connected",
		}
		storageNode.SendResponse(response)
		diskSpaceMap[machineName+":"+port] = diskSpace
	} else {
		//present in the map get time difference and if greater than 25 reject
		previousTime := serverName_time[machineName][port]
		diff := currentTime.Sub(previousTime)
		//fmt.Println(machineName, ":", port, " :Last beat was sent ", diff, " seconds ago.")
		if math.Round(float64(diff/time.Second)) > 25 {
			//send response to servers with above message
			response := &storageNode_handler.Response{
				Message: "ERROR: Time Exceeded! Please re-initialize the server.",
			}
			storageNode.SendResponse(response)
		} else {
			serverName_time[machineName][port] = currentTime
			//send response to servers that with acceptance of the beat
			response := &storageNode_handler.Response{
				Message: "SUCCESS: HeartBeat accepted. Still connected",
			}
			storageNode.SendResponse(response)
			diskSpaceMap[machineName+":"+port] = diskSpace
		}
	}

}

func storageNode_ControllerListener(wg *sync.WaitGroup) {
	defer wg.Done()
	//central component should run on orion02:4000
	listener, err := net.Listen("tcp", ":"+"4000")
	if err != nil {
		log.Fatalln(err.Error())
		return
	} else {
		//fmt.Println("Central Component started on port for SN:: ", 4000)

		for {
			if conn, err := listener.Accept(); err == nil {
				//get response from client i.e client serverName, port and beat
				storageNode := storageNode_handler.NewStorageNode(conn)
				//perform operations
				performOperations(storageNode)
				conn.Close()
			}
		}
	}
}

func main() {
	var wg sync.WaitGroup
	go manageReplication()
	wg.Add(2)

	go storageNode_ControllerListener(&wg)
	go client_ControllerListener(&wg)

	wg.Wait()
}

/*
	New functionality
	Listening on port to receive file from client
*/
func client_ControllerListener(wg *sync.WaitGroup) { //changed
	defer wg.Done()
	listener, err := net.Listen("tcp", ":"+"5000")
	helper.CheckErr(err)
	fmt.Println("Central Component started on port for Client:: ", 5000)
	destination, err := os.Getwd()
	helper.CheckErr(err)
	for {
		if conn, err := listener.Accept(); err == nil {
			clientHandler := client_handler.NewFileHandler(conn)
			handleClientMsg(clientHandler, destination)
		}
	}

}

func manageReplication() {
	for {
		inactiveNodes := getInActiveStorageNodes()
		activeNodes := getActiveStorageNodes()
		if len(inactiveNodes) > 0 {
			fmt.Println("Inactive nodes found")
			//For each inactive node
			for _, inactiveNode := range inactiveNodes {
				fmt.Println("Handling ", inactiveNode, "'s data replication")
				//Get files required to replicate
				filesToReplicate := fileLocationMap[inactiveNode]
				fmt.Println("Files to replicate  ", filesToReplicate)
				//For each file in file list
				for _, file := range filesToReplicate {
					//Find all the locations for file
					newReplicaFileName, newMachineName, activeReplicaFileName, activeReplicaLocation := updateReplicaStorageNodeMap(file, inactiveNode, activeNodes)
					fmt.Println("Replica went down. Details -> ", inactiveNode, " files to replicate ", newReplicaFileName, " .Getting file replica from ", activeReplicaLocation, " sending file to ", newMachineName)
					if newReplicaFileName != "" {
						updateFileStorageNodeMap(file, inactiveNode, newMachineName)
						//Send message to storage node  X that send file abc to storage node Y
						askStorageNodeToReplicateData(activeReplicaFileName, newReplicaFileName, activeReplicaLocation, newMachineName)
					}

				}
			}
		}
		fmt.Println("Sleeping for 5 seconds")
		time.Sleep(5 * time.Second)
	}
}
func askStorageNodeToReplicateData(activeReplicaFileName string, filename string, activeReplicaLocation string, newMachineName string) {
	machineName := strings.Split(activeReplicaLocation, ":")[0]
	conn, err := net.Dial("tcp", machineName+":9999")
	helper.CheckErr(err)
	clientHandler := client_handler.NewFileHandler(conn)
	clientHandler.Send(&client_handler.ClientMsg{Status: "Replicate:" + newMachineName + ":" + activeReplicaFileName, Filename: filename})
}
func updateReplicaStorageNodeMap(filename string, inactiveNode string, activeNodes []string) (string, string, string, string) {
	replicaList := replicaStorageNodeMap[filename]
	activeReplicaLocation := ""
	activeReplicaFileName := ""
	newFilename := ""
	newMachineName := ""
	if replicaList != nil {
		replicaSlice := replicaList.ReplicaList
		fmt.Println("Replica list -> ", replicaList)
		//We know if we need to name the file with _r or not
		newIndex := rand.Intn(len(activeNodes) - 1)
		newMachineName = activeNodes[newIndex%(len(activeNodes))]

		i := 0
		for index, replica := range replicaSlice {
			if index == 0 && replica == inactiveNode {
				newFilename = filename
				activeReplicaFileName = filename + "_r"
				activeReplicaLocation = replicaSlice[index+1]
				i = index
				break
			} else if index == 1 && replica == inactiveNode {
				newFilename = filename + "_r"
				activeReplicaFileName = filename
				activeReplicaLocation = replicaSlice[index-1]
				i = index
				break
			} else if index == 2 && replica == inactiveNode {
				newFilename = filename + "_r"
				activeReplicaFileName = filename + "_r"
				activeReplicaLocation = replicaSlice[index-1]
				i = index
				break
			}
		}

		replicaSlice[i] = newMachineName
		replicaStorageNodeMap[filename] = replicaList
	}
	return newFilename, newMachineName, activeReplicaFileName, activeReplicaLocation
}
func updateFileStorageNodeMap(filename string, inactiveNode string, newMachineName string) {
	originalFilename := strings.TrimRight(filename, "_")
	chunkMap := fileStorageNodeMap[originalFilename]
	machineNameWithPort := chunkMap[filename]
	machineDetails := strings.Split(machineNameWithPort, ":")
	if machineDetails[0] == inactiveNode {
		chunkMap[filename] = newMachineName
		fileStorageNodeMap[originalFilename] = chunkMap
	}
}

func getInActiveStorageNodes() []string {
	var inactiveNodes []string
	for machine, machineHeartbeatMap := range serverName_time {
		for port, heartbeat := range machineHeartbeatMap {
			currentTime := time.Now()
			diff := currentTime.Sub(heartbeat)
			if diff/time.Second > 25 {
				inactiveNodes = append(inactiveNodes, machine+":"+port)
			}
		}
	}
	fmt.Println("inActiveNodes:", inactiveNodes)
	return inactiveNodes
}

func getActiveStorageNodes() []string {
	var activeNodes []string
	for machine, machineHeartbeatMap := range serverName_time {
		for port, heartbeat := range machineHeartbeatMap {
			currentTime := time.Now()
			diff := currentTime.Sub(heartbeat)
			if diff/time.Second < 25 {
				activeNodes = append(activeNodes, machine+":"+port)
			}
		}
	}
	fmt.Println("ActiveNodes:", activeNodes)
	return activeNodes
}

func handleClientMsg(clientHandler *client_handler.FileHandler, destination string) {
	// fmt.Println("File transfer started")
	response, err := clientHandler.Receive()
	helper.CheckErr(err)
	action := response.GetAction()
	fmt.Println("action::", action)
	switch {
	case action == "put":
		fmt.Println("Sending chunk details to client")
		// if !helper.CheckFileExists(destination + "/" + response.GetFilename()) /*&& checkEnoughSpaceAvailable(response.GetFilesize(), destination)*/ {
		//putFile(clientHandler, destination)
		clientHandler.Send(&client_handler.ClientMsg{Status: "Ok"})
		response, err := clientHandler.Receive()
		helper.CheckErr(err)
		//Vinay's work
		_, exist := fileStorageNodeMap[response.GetFilename()]
		if !exist {
			//Write a function which returns list of active storage nodes
			activeNodes := getActiveStorageNodes() //["orion01","orion02","orion03"] /*What if there are no active node? handle no storage node condition.*/
			//store chunk from startIndex which is calculated in gandhar's code upto n number of chunks in a map and send the values to the client.

			/*Logic to send storage node names to client */ //Gandhar's work

			/* 1. Generate a random index from given storage nodes */
			startIndex := rand.Intn(len(activeNodes) - 1)

			/* 2. Calculate no of chunks required to store files total mb/ size of chunk in mb */
			requiredChunks := (response.GetFilesize() / (response.GetChunksize() * 1000000)) + 1
			fmt.Println("RequiredChunk::", requiredChunks)
			/* 3. Finalize list of nodes to return */
			var chunkMap = make(map[string]string)
			var chunkIndex = 1
			index := startIndex
			for i := requiredChunks; i > 0; i-- {
				/* Add machinename at index to a list */
				key := response.GetFilename() + "_" + strconv.Itoa(chunkIndex)
				// fmt.Println("Nodeeeeeee:", activeNodes[index%(len(activeNodes))])
				chunkMap[key] = activeNodes[index%(len(activeNodes))]

				/* Logic to find next node and next to next node for replication */
				replicaSlice := make([]string, 3)
				replicaSlice[0] = activeNodes[(index)%(len(activeNodes))]
				replicaSlice[1] = activeNodes[(index+1)%(len(activeNodes))]
				replicaSlice[2] = activeNodes[(index+2)%(len(activeNodes))]

				/* Node with original chunk */
				replicaStorageNodeMap[key] = &client_handler.ClientMsgReplicaNodes{ReplicaList: replicaSlice}

				/* Adding details into file map*/
				fileLocationMap[activeNodes[index%(len(activeNodes))]] = append(fileLocationMap[activeNodes[index%(len(activeNodes))]], key)
				fileLocationMap[activeNodes[(index+1)%(len(activeNodes))]] = append(fileLocationMap[activeNodes[(index+1)%(len(activeNodes))]], key)
				fileLocationMap[activeNodes[(index+2)%(len(activeNodes))]] = append(fileLocationMap[activeNodes[(index+2)%(len(activeNodes))]], key)

				index += 1
				chunkIndex += 1
			}
			/* 4. Store {filename, storagenode list} in a Map */
			fileStorageNodeMap[response.GetFilename()] = chunkMap
			fmt.Println("Chunk::", chunkMap)
			/* 5. Find replication locations for each chunk */

			/* 6. Update replicaStorageNode Map */

			// 7. Send chunk map & replica map to client
			clientHandler.Send(&client_handler.ClientMsg{ChunkDetails: fileStorageNodeMap[response.GetFilename()], ReplicaDetails: replicaStorageNodeMap, Status: "SUCESS: Chunk Details sent ot he client."}) //**Changed on 30march at 4:51am. Add field("chunkDetails") to clientMsg having type map<string,string>

		} else {
			clientHandler.Send(&client_handler.ClientMsg{ChunkDetails: make(map[string]string), Status: "Error: File already exist."})
		}
	case action == "get":
		fmt.Printf("Getting file")
		_, exist := fileStorageNodeMap[response.GetFilename()]
		if exist {
			/* Logic to send storage node names to client to retrieve the file */
			fmt.Println("Downloading file")

			filename := response.GetFilename()

			//Filtered map containing subset of replicaStorageNodeMap
			replicaMap := make(map[string]*client_handler.ClientMsgReplicaNodes)

			//Filter chunks of specific file only
			for key, val := range replicaStorageNodeMap {
				if strings.Contains(key, filename) {
					replicaMap[key] = val
				}
			}
			clientHandler.Send(&client_handler.ClientMsg{ChunkDetails: fileStorageNodeMap[response.GetFilename()], ReplicaDetails: replicaMap, Status: "SUCCESS: Chunk Details sent to the client."})
			//getFile(clientHandler, response.GetFilename())
		} else {
			clientHandler.Send(&client_handler.ClientMsg{Status: "ERROR: File does not exist"})
		}
	case action == "ls":
		fmt.Println("Sending file list to client")
		fileList := make([]string, 0)
		for file := range fileStorageNodeMap {
			fileList = append(fileList, file)
		}
		// activeNodes := getActiveStorageNodes()
		// for _, node := range activeNodes {
		// 	// machineName, port := strings.Split(node, ":")
		// 	// machineName
		// }
		clientHandler.Send(&client_handler.ClientMsg{FileList: fileList, DiskSpaceMap: diskSpaceMap})
	case action == "delete":
		fmt.Println("IN delete ")
		// filename := response.GetFilename()
		clientHandler.Send(&client_handler.ClientMsg{ReplicaDetails: replicaStorageNodeMap})
		// var fileStorageNodeMap = make(map[string]map[string]string)                        /*Map{FileName {chunk name, storage node name with port number seperated by colon}}*/
		// var replicaStorageNodeMap = make(map[string]*client_handler.ClientMsgReplicaNodes) //{chunkName (file.txt_1):{ ["orion05","orion06","orion07"]} } (orion05 original SN)(orion06 and orion07 duplicate SN)
		//TODO::delete from Map
		delete(fileStorageNodeMap, response.GetFilename())
		for chunkName := range replicaStorageNodeMap {
			if strings.Contains(chunkName, response.GetFilename()) {
				delete(replicaStorageNodeMap, chunkName)
			}
		}

	default:
		clientHandler.Send(&client_handler.ClientMsg{Status: "ERROR: Unexpected action. Please choose between get/put"})
	}
	clientHandler.Close()
}

/*
func putFile(clientHandler *client_handler.FileHandler, destination string) {
	clientHandler.Send(&client_handler.ClientMsg{Status: "Ok"})
	response, err := clientHandler.Receive()
	err = os.WriteFile(destination+"/"+response.GetFilename(), response.GetData(), 0644)
	helper.CheckErr(err)
	if response.GetChecksum() != helper.GetFileCheckSum(destination+"/"+response.GetFilename()) {
		clientHandler.Send(&client_handler.ClientMsg{Status: "Err: Checksum mismatch"})
		err = os.Remove(destination + "/" + response.GetFilename())
		helper.CheckErr(err)
	}

}
*/
