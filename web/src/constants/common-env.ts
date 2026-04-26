const isDev = import.meta.env?.DEV ?? false;

const webConfig = {
  apiUrl: isDev ? "http://127.0.0.1:7000" : "",
};

export default webConfig;
