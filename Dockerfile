FROM scratch
COPY united /united
VOLUME ["/pb_data"]
ENTRYPOINT ["/united", "serve", "--dir=/pb_data"]
