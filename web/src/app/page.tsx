"use client";

import { useEffect, useState } from "react";
import { Navigate } from "react-router-dom";

import { STARTUP_CHECK_COMPLETED_KEY } from "@/constants/startup-check";

export default function HomePage() {
  const [isReady, setIsReady] = useState(false);
  const [isCompleted, setIsCompleted] = useState(false);

  useEffect(() => {
    try {
      setIsCompleted(localStorage.getItem(STARTUP_CHECK_COMPLETED_KEY) === "1");
    } catch {
      setIsCompleted(false);
    } finally {
      setIsReady(true);
    }
  }, []);

  if (!isReady) {
    return null;
  }
  return <Navigate to={isCompleted ? "/image" : "/startup-check"} replace />;
}
