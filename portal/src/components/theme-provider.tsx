"use client";

import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { Toaster } from "sonner";

type ThemeMode = "light" | "graphite" | "dark";

type ThemeContextValue = {
  themeMode: ThemeMode;
  isDark: boolean;
  isDarkLike: boolean;
  setThemeMode: (mode: ThemeMode) => void;
  toggleTheme: () => void;
};

const THEME_STORAGE_KEY = "chatgpt-image-studio:theme";

const ThemeContext = createContext<ThemeContextValue | null>(null);

function normalizeThemeMode(value: string | null | undefined): ThemeMode {
  if (value === "dark" || value === "graphite") {
    return value;
  }
  return "light";
}

function readStoredThemeMode(): ThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }
  try {
    return normalizeThemeMode(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    return "light";
  }
}

function applyThemeMode(mode: ThemeMode) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  const isDarkLike = mode !== "light";
  root.classList.toggle("dark", isDarkLike);
  root.classList.toggle("theme-dark", mode === "dark");
  root.classList.toggle("theme-graphite", mode === "graphite");
  root.dataset.theme = mode;
  root.style.colorScheme = isDarkLike ? "dark" : "light";
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeMode, setThemeModeState] = useState<ThemeMode>(() =>
    readStoredThemeMode(),
  );

  useLayoutEffect(() => {
    applyThemeMode(themeMode);
  }, [themeMode]);

  useEffect(() => {
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, themeMode);
    } catch {
      // Keep the in-memory theme when browser storage is unavailable.
    }
  }, [themeMode]);

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== THEME_STORAGE_KEY) {
        return;
      }
      setThemeModeState(normalizeThemeMode(event.newValue));
    };

    window.addEventListener("storage", handleStorage);
    return () => {
      window.removeEventListener("storage", handleStorage);
    };
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({
      themeMode,
      isDark: themeMode === "dark",
      isDarkLike: themeMode !== "light",
      setThemeMode: (mode) => {
        setThemeModeState((current) => (current === mode ? current : mode));
      },
      toggleTheme: () => {
        setThemeModeState((current) => {
          if (current === "light") {
            return "graphite";
          }
          if (current === "graphite") {
            return "dark";
          }
          return "light";
        });
      },
    }),
    [themeMode],
  );

  return (
    <ThemeContext.Provider value={value}>
      <Toaster
        position="top-center"
        richColors
        theme={themeMode === "light" ? "light" : "dark"}
      />
      {children}
    </ThemeContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error("useTheme must be used within ThemeProvider");
  }
  return context;
}
