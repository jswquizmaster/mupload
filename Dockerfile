FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY mupload-server .
EXPOSE 8080
VOLUME ["/uploads"]
ENV UPLOAD_DIR=/uploads
CMD ["./mupload-server"]
