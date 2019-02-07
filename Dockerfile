FROM ubuntu:18.04
MAINTAINER Luca Moser <moser.luca@gmail.com>

# create app dir
RUN mkdir -p /app

# copy server assets
COPY config.json /app/config.json
COPY confbox /app/confbox

# workdir and ports
WORKDIR /app
EXPOSE 9090

# entrypoint
ENTRYPOINT ["/app/confbox"]