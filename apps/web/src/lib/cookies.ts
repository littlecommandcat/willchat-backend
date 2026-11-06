import Cookies from "universal-cookie";

const cookies = new Cookies();
const options = { path: "/", sameSite: "strict" as const };

export const getToken = (): string | undefined => cookies.get("token", { doNotParse: true });
export const setToken = (token: string) => cookies.set("token", token, {
  ...options,
  secure: window.location.protocol === "https:",
});
export const clearToken = () => cookies.remove("token", options);