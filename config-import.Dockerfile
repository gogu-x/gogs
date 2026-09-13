FROM python:3.11.11-alpine3.20

WORKDIR /app
COPY gogs/tools/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt
COPY gogs/tools/ /app/tools/
COPY glconf/ /glconf/

ENTRYPOINT ["python", "/app/tools/import-conf.py"]
