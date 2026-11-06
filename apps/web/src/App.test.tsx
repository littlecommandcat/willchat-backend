import { act, fireEvent, render, screen } from "@testing-library/react";
import App from "./App";
import { clearToken, getToken } from "./lib/cookies";

const originalFetch = global.fetch;
const originalWebSocket = global.WebSocket;

afterEach(() => {
  clearToken();
  global.fetch = originalFetch;
  global.WebSocket = originalWebSocket;
});

test("opens registration and returns to login", () => {
  render(<App />);
  fireEvent.click(screen.getByRole("link", { name: "立即註冊" }));
  expect(screen.getByRole("heading", { name: "建立帳號" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("link", { name: "返回登入" }));
  expect(screen.getByRole("button", { name: "登入" })).toBeInTheDocument();
});

test("logs in, loads history, receives a message, and logs out", async () => {
  const member = { id: "alice", name: "Alice", groups: [{ group_id: "chat", name: "聊天群組" }] };
  const socket = { onmessage: (_event: { data: string }) => {}, close: jest.fn() };
  const websocket = jest.fn((_url: URL) => socket);
  global.WebSocket = websocket as unknown as typeof WebSocket;
  const fetchMock = jest.fn(async (url: string, _options: RequestInit) => ({
    ok: true,
    json: async () => ({ status: "ok", data: url === "/auth/login"
      ? { token: "test-token", member }
      : url.includes("/messages") ? { messages: [] } : { member } }),
  }));
  global.fetch = fetchMock as unknown as typeof fetch;

  render(<App />);
  fireEvent.change(screen.getByLabelText("電子郵件"), { target: { value: "alice@example.com" } });
  fireEvent.change(screen.getByLabelText("密碼"), { target: { value: "password123" } });
  fireEvent.click(screen.getByRole("button", { name: "登入" }));
  const group = await screen.findByText("聊天群組");
  expect(getToken()).toBe("test-token");
  expect((fetchMock.mock.calls.find(([url]) => url === "/api/me")?.[1].headers as Headers)
    .get("Authorization")).toBe("test-token");
  expect(String(websocket.mock.calls[0]?.[0])).toContain("/ws?token=test-token");

  fireEvent.click(group);
  await act(async () => {});
  expect(fetchMock).toHaveBeenCalledWith("/api/groups/chat/messages?amount=50", expect.objectContaining({ method: "GET" }));
  act(() => socket.onmessage({ data: JSON.stringify({ event: "message_send", payload: {
    group: { group_id: "chat" }, message: { id: "1", authorid: "alice", content: "你好", timestamp: 1 },
  } }) }));
  expect(screen.getByText("你好")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "登出" }));
  expect(getToken()).toBeUndefined();
  expect(socket.close).toHaveBeenCalled();
});
