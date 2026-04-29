const proxyTarget = import.meta.env.VITE_API_PROXY_TARGET?.trim();
const directApiUrl = import.meta.env.VITE_API_URL?.trim();

const webConfig = {
  apiUrl: import.meta.env.DEV ? (proxyTarget ? "" : directApiUrl || "http://127.0.0.1:7000") : "",
};

export default webConfig;
