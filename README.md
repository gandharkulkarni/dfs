# Project 1: Distributed File System


**A. COMMUNICATION FLOW DIAGRAM**

**PUT**<br/>
<img width="793" alt="Screen Shot 2023-04-06 at 1 21 35 AM" src="https://user-images.githubusercontent.com/109916498/230320276-6d72b2f8-3d4d-47dd-8e5b-8b3521418b8f.png">

**GET**<br/>
<img width="738" alt="Screen Shot 2023-04-06 at 1 21 52 AM" src="https://user-images.githubusercontent.com/109916498/230320248-6d141435-7ca0-4262-bb8d-a23e51e06135.png">

**DELETE**<br/>
<img width="725" alt="Screen Shot 2023-04-06 at 1 22 20 AM" src="https://user-images.githubusercontent.com/109916498/230320186-3b401dba-1687-4ae8-a9a5-ea7aef03cdac.png">




**B. PROTO**

**1. CONTROLLER <-> STORAGE NODE**

    message beat{
        string machineName = 1;
        string port = 2;
        bool beat = 3;
        string diskSpace = 4;
    }

    message response{
        string message = 1;
    }

**2. CONTROLLER <-> CLIENT**

    message clientMsg{
        bytes data = 1;
        string filename = 2;
        string action = 3;
        string checksum = 4;
        string status = 5;
        int64 filesize = 6;
        int64 chunksize = 7;
        map<string, string> chunkDetails = 8;
        map<string, replicaNodes> replicaDetails = 9;
        message replicaNodes{
            repeated string replicaList = 1;
        }
        repeated string fileList = 10;
        map<string, string> diskSpaceMap = 11;
    }


**C. COMMAND LINE INTERFACE**

**1. CLIENT INTERFACE**
    
Action (PUT/GET/DELETE)

    ./client put FILEPATH chunkSize //Put file

    ./client get FILENAME DESTINATION_PATH //Get file

    ./client delete FILENAME

    ./client ls


**2. CONTROLLER INTERFACE**

    ./controller <Port>

**3. STORAGE NODE INTERFACE**

    ./storage_node <Machinename:port>
