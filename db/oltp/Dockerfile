FROM postgres:18.7-alpine

WORKDIR /app

COPY sql/ /app/sql/
COPY migrate.sh /app/migrate.sh
RUN chmod +x /app/migrate.sh

ENTRYPOINT ["/app/migrate.sh"]
