"use client";

import { cn } from "@/lib/utils";

function buildInitials(name?: string, email?: string) {
  const source = String(name || "").trim() || String(email || "").trim();
  if (!source) {
    return "CS";
  }

  const normalized = source.includes("@") ? source.split("@")[0] : source;
  const words = normalized
    .split(/[\s._-]+/)
    .map((item) => item.trim())
    .filter(Boolean);

  if (words.length >= 2) {
    return `${words[0][0] ?? ""}${words[1][0] ?? ""}`.toUpperCase();
  }

  const runes = Array.from(normalized);
  if (runes.length >= 2) {
    return `${runes[0]}${runes[1]}`.toUpperCase();
  }
  return (runes[0] || "C").toUpperCase();
}

export function PortalAvatar({
  src,
  name,
  email,
  className,
  textClassName,
}: {
  src?: string;
  name?: string;
  email?: string;
  className?: string;
  textClassName?: string;
}) {
  const value = String(src || "").trim();
  const initials = buildInitials(name, email);

  return (
    <span
      className={cn(
        "inline-flex items-center justify-center overflow-hidden rounded-full bg-stone-200 text-stone-700 shadow-sm",
        className,
      )}
    >
      {value ? (
        <img src={value} alt={name || email || "avatar"} className="h-full w-full object-cover" />
      ) : (
        <span className={cn("text-xs font-semibold uppercase", textClassName)}>{initials}</span>
      )}
    </span>
  );
}
