export interface JobStatus {
  status: string;
  message: string;
  data?: Record<string, string>;
  progress: number;
  operation?: string;
}

export function waitForJobWS(
  jobId: string,
  onUpdate: (st: JobStatus) => void
): Promise<void> {
  return new Promise((resolve) => {
    let resolved = false;
    const safeResolve = () => {
      if (!resolved) {
        resolved = true;
        resolve();
      }
    };

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(
      `${proto}//${window.location.host}/api/status/ws/${jobId}`
    );
    
    ws.onmessage = (ev) => {
      try {
        const st = JSON.parse(ev.data);
        onUpdate(st);
        if (st.status === "success" || st.status === "error") {
          // Add small delay to allow state update to complete and
          // prevent "Close received after close" race condition
          setTimeout(() => {
            ws.close();
            safeResolve();
          }, 10);
          return;
        }
      } catch {}
    };

    ws.onclose = () => safeResolve();
    ws.onerror = () => safeResolve();
  });
}
