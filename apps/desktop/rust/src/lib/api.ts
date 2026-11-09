import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

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
  author_id: string;
  author_name?: string;
  content: string;
  timestamp: number;
  type: number;
  attachments?: { data?: string; mime_type?: string }[];
  author?: Member;
  reply_to?: { author?: Member; authorid?: string; content?: string } | null;
  status?: string;
  group?: { id: string; name: string } | null;
  __pending__?: boolean;
}

export interface ApiResult {
  ok: boolean;
  error?: string;
  code?: string;
  [key: string]: unknown;
}

export function invokeError(error: unknown, fallback: string): ApiResult {
  const message = error instanceof Error ? error.message : String(error || "");
  return { ok: false, error: message || fallback };
}

export const api = {
  getWebsocketStatus: () => invoke<string>("get_websocket_status"),
  updateProfile: (name: string) => invoke<ApiResult>("update_profile", { name }),
  resetPassword: (password: string) => invoke<ApiResult>("reset_password", { password }),
  health: () => invoke<ApiResult>("health"),
  login: (email: string, password: string) =>
    invoke<ApiResult>("login", { email, password }),
  register: (username: string, email: string, password: string) =>
    invoke<ApiResult>("register", { username, email, password }),
  getState: () =>
    invoke<{ member: Member; groups: Group[] }>("get_state"),
  getMessages: (groupId: string) =>
    invoke<ApiResult & { messages?: Message[] }>("get_messages", { groupId }),
  getMembers: (groupId: string) =>
    invoke<{ members: Member[] }>("get_members", { groupId }),
  sendMessage: (groupId: string, content: string) =>
    invoke<ApiResult>("send_message", { groupId, content }),
  createGroup: (name: string) => invoke<ApiResult>("create_group", { name }),
  deleteGroup: (groupId: string) =>
    invoke<ApiResult>("delete_group", { groupId }),
  joinGroup: (groupId: string) => invoke<ApiResult>("join_group", { groupId }),
  leaveGroup: (groupId: string) =>
    invoke<ApiResult>("leave_group", { groupId }),
  navigate: (page: string) => invoke<void>("navigate", { page }),
};

export interface ServerEvent {
  type: string;
  [key: string]: unknown;
}

export const onServerEvent = (handler: (event: ServerEvent) => void) =>
  listen<ServerEvent>("willchat:event", (event) => handler(event.payload));
