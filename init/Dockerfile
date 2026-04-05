FROM alpine:3.21
RUN apk add --no-cache openssl
COPY generate-secrets.sh /generate-secrets.sh
RUN sed -i 's/\r$//' /generate-secrets.sh
RUN chmod +x /generate-secrets.sh
ENTRYPOINT ["/generate-secrets.sh"]
