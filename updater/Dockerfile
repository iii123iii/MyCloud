FROM docker:27-cli

RUN apk add --no-cache \
    docker-cli-compose \
    git \
    bash \
    python3 \
    mariadb-client \
    mariadb-connector-c

WORKDIR /updater
COPY server.py /updater/server.py
COPY update.sh /updater/update.sh
RUN chmod +x /updater/update.sh

EXPOSE 9999

CMD ["python3", "/updater/server.py"]
