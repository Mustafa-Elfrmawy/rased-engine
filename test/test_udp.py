import socket
import json
import time

# إعدادات السيرفر
UDP_IP = "127.0.0.1"
UDP_PORT = 8070

# الداتا اللي هنجرب نبعتها (نفس اللي الـ ESP بيبعتها)
payload = {
    "id": "ESP_TRACKER_001",
    "lat": 30.582,
    "lng": 30.921,
    "alt": 10.5,
    "spd": 0.5,
    "crs": 12.3,
    "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "sat": 12,
    "hdop": 1.1,
    "sig": "22",
    "icc": 0,
    "gia": 1,
    "val": 1,
    "wkp": 0
}

print(f"🎯 Sending UDP data to {UDP_IP}:{UDP_PORT}")

# تجهيز الـ Socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.settimeout(3.0)  # وقت الانتظار 3 ثواني للرد

message = json.dumps(payload).encode('utf-8')
print(f"📦 Payload: {message.decode('utf-8')}")

# إرسال البيانات
sock.sendto(message, (UDP_IP, UDP_PORT))

# استقبال الرد (ACK)
try:
    data, addr = sock.recvfrom(1024)
    print(f"✅ Received response: {data.decode('utf-8')} from {addr}")
except socket.timeout:
    print("❌ Timeout: No response received from the server.")

sock.close()
