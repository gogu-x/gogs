conf:
	cd /data/


sudo docker run -d --name v2ray4  --restart unless-stopped -v /etc/v2ray/config.json:/data/config.json -p 8086:8086    v2fly/v2fly-core:latest run -c /etc/v2ray/config.json