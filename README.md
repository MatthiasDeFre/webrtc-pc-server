# WebRTC based Point Cloud Streaming


# Project Structure


# Building


# Dependencies


# Usage
Following command line parameters can be used to change the behaviour of the application:

| **Parameter** | **Name**           | **Description**                                                          | **Example**    |
|---------------|--------------------|--------------------------------------------------------------------------|----------------|
| -v            | IP Filter          | Forces the server to communicate using the interface with this IP        | 192.168.10.1   |
| -p            | Proxy Port         | If enabled the server will receive frames from a UDP socket on this port | :8001          |
| -d            | Content Directory  | When not using the proxy port, a folder with content can be used instead | content_madfr  |
| -f            | Content Frame Rate | When not using the proxy port, the FPS at which the content is server    | 30             |
| -s            | Signaling IP       | IP on which the signaling server will be created                         | 127.0.0.1:5678 |
| -m            | Result Path        | The path to which metrics are saved (folder + file without extension)    | results/exp_1  |

# Roadmap
