FROM ubuntu:20.04
RUN apt-get update && apt-get install python3 -y &&\
    apt-get install python3-pip -y && pip3 install Flask==2.3.2 openai==0.27.8