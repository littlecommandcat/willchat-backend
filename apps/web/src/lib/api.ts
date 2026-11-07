import { getToken } from "./cookies";

export interface Member {
  id: string;
  name: string;
}

export interface Group {
  id: string;
  name: string;
  owner_id: string;
  members: Member[];
}

export interface Message {
  id: string;
  authorid: string;
  content: string;
  timestamp: number;
  attachments?: { data?: string; mime_type?: string }[];
  reply_to?: { authorid?: string; content?: string };
}

interface GroupRecord {
  group_id: string;
  name?: string;
  owner_id?: string;
}

export interface Account extends Member {
  groups?: GroupRecord[];
}

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string; code?: string };
const baseUrl = (process.env.REACT_APP_API_URL || "").replace(/\/+$/, "");

async function request<T = unknown>(path: string, method = "GET", body?: object): Promise<ApiResult<T>> {
  const headers = new Headers();
  const token = getToken();
  if (token) headers.set("Authorization", token);
  if (body) headers.set("Content-Type", "application/json");

  try {
    const response = await fetch(`${baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    const result = await response.json().catch(() => null);
    if (!response.ok || result?.status !== "ok") {
      return {
        ok: false,
        code: result?.code,
        error: result?.message || `請求失敗 (${response.status})`,
      };
    }
    return { ok: true, data: result.data };
  } catch {
    return { ok: false, error: "無法連接伺服器" };
  }
}

const groupPath = (id: string) => `/api/groups/${encodeURIComponent(id)}`;

export const api = {
  login: (email: string, password: string) =>
    request<{ token: string; member: Account }>("/auth/login", "POST", { email, password }),
  register: (name: string, email: string, password: string) =>
    request("/auth/register", "POST", { name, email, password }),
  getState: () => request<{ member: Account }>("/api/me"),
  updateProfile: (name: string) => request("/api/me", "PUT", { name }),
  resetPassword: (password: string) => request("/auth/reset", "POST", { password }),
  getMessages: (id: string) => request<{ messages: Message[] }>(`${groupPath(id)}/messages?amount=50`),
  getMember: (groupId: string, memberId: string) =>
    request<{ member: Member }>(`${groupPath(groupId)}/members/${encodeURIComponent(memberId)}`),
  sendMessage: (id: string, content: string) =>
    request<{ message: Message }>(`${groupPath(id)}/messages`, "POST", { content }),
  createGroup: (name: string) => request("/api/groups", "POST", { name }),
  deleteGroup: (id: string) => request(groupPath(id), "DELETE"),
  joinGroup: (id: string) => request(`${groupPath(id)}/members`, "POST"),
  leaveGroup: (id: string) => request(`${groupPath(id)}/members`, "DELETE"),
};

export type ConnectionStatus = "closed" | "connecting" | "connected";

export interface ServerEvent {
  event: string;
  payload?: {
    member?: Account;
    group?: GroupRecord;
    message?: Message;
    online?: string[];
  };
}

export function connectEvents(
  token: string,
  onEvent: (event: ServerEvent) => void,
  onStatus: (status: ConnectionStatus) => void,
) {
  const url = new URL(`${baseUrl}/ws`, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("token", token);
  let stopped = false;
  let socket: WebSocket;
  let retry: ReturnType<typeof setTimeout>;

  function connect() {
    onStatus("connecting");
    socket = new WebSocket(url);
    socket.onopen = () => { if (!stopped) onStatus("connected"); };
    socket.onmessage = ({ data }) => {
      if (stopped || typeof data !== "string") return;
      let event: ServerEvent;
      try { event = JSON.parse(data); } catch { return; }
      if (typeof event?.event === "string") onEvent(event);
    };
    socket.onclose = () => {
      if (stopped) return;
      onStatus("connecting");
      retry = setTimeout(connect, 3000);
    };
  }

  connect();
  return () => {
    stopped = true;
    clearTimeout(retry);
    socket.close();
  };
}