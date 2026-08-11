#!/usr/bin/env python
import psycopg

def healthcheck():
    try:
        conn = psycopg.connect("postgresql://tshark_user:tshark_password@postgres:5432/tshark_db")
        conn.execute("SELECT 1")
        conn.close()
        return True
    except Exception:
        return False

if __name__ == "__main__":
    import sys
    sys.exit(0 if healthcheck() else 1)