export type ImageAccountSortMode = "imported_at" | "name" | "quota";
export type ImageAccountReserveMode = "daily_first_seen_percent";

export type StoredImageAccountPolicy = {
  enabled: boolean;
  sortMode: ImageAccountSortMode;
  groupSize: number;
  enabledGroupIndexes: number[];
  reserveMode: ImageAccountReserveMode;
  reservePercent: number;
};

export type ImagePolicyAccountLike = {
  id: string;
  fileName: string;
  email?: string | null;
  importedAt?: string | null;
  quota: number;
  status: string;
  restoreAt?: string | null;
  limits_progress?: Array<{
    feature_name?: string;
    remaining?: number;
    reset_after?: string;
  }>;
};

export type ImageAccountGroupPreview = {
  index: number;
  label: string;
  enabled: boolean;
  accounts: ImagePolicyAccountLike[];
  availableCount: number;
  totalRemaining: number;
  averageRemaining: number;
};

const STORAGE_KEY = "studio.image-account-policy.v1";

const defaultPolicy: StoredImageAccountPolicy = {
  enabled: true,
  sortMode: "imported_at",
  groupSize: 10,
  enabledGroupIndexes: [0, 1],
  reserveMode: "daily_first_seen_percent",
  reservePercent: 20,
};

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function normalizeGroupIndexes(groupIndexes: number[]) {
  return Array.from(
    new Set(
      groupIndexes
        .map((value) => Number(value))
        .filter((value) => Number.isInteger(value) && value >= 0),
    ),
  ).sort((left, right) => left - right);
}

export function normalizeImageAccountPolicy(
  value: Partial<StoredImageAccountPolicy> | null | undefined,
): StoredImageAccountPolicy {
  const sortMode = value?.sortMode;
  const nextSortMode: ImageAccountSortMode =
    sortMode === "name" || sortMode === "quota" ? sortMode : "imported_at";
  const enabledGroupIndexes = normalizeGroupIndexes(value?.enabledGroupIndexes ?? defaultPolicy.enabledGroupIndexes);

  return {
    enabled: value?.enabled ?? defaultPolicy.enabled,
    sortMode: nextSortMode,
    groupSize: clamp(Number(value?.groupSize) || defaultPolicy.groupSize, 1, 100),
    enabledGroupIndexes,
    reserveMode: "daily_first_seen_percent",
    reservePercent: clamp(Number(value?.reservePercent) || defaultPolicy.reservePercent, 0, 100),
  };
}

export function getDefaultImageAccountPolicy() {
  return { ...defaultPolicy };
}

export function getStoredImageAccountPolicy() {
  if (typeof window === "undefined") {
    return getDefaultImageAccountPolicy();
  }
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return getDefaultImageAccountPolicy();
    }
    const parsed = JSON.parse(raw) as Partial<StoredImageAccountPolicy>;
    return normalizeImageAccountPolicy(parsed);
  } catch {
    return getDefaultImageAccountPolicy();
  }
}

export function setStoredImageAccountPolicy(policy: StoredImageAccountPolicy) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(normalizeImageAccountPolicy(policy)));
}

function encodeBase64Url(raw: string) {
  const bytes = new TextEncoder().encode(raw);
  let binary = "";
  bytes.forEach((value) => {
    binary += String.fromCharCode(value);
  });
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

export function buildImageAccountPolicyHeader(policy = getStoredImageAccountPolicy()) {
  const normalized = normalizeImageAccountPolicy(policy);
  if (!normalized.enabled) {
    return "";
  }
  return encodeBase64Url(JSON.stringify(normalized));
}

function parseDateValue(value?: string | null) {
  if (!value) {
    return Number.POSITIVE_INFINITY;
  }
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : Number.POSITIVE_INFINITY;
}

function currentImageRemaining(account: ImagePolicyAccountLike) {
  const imageGen = account.limits_progress?.find((item) => item.feature_name === "image_gen");
  if (typeof imageGen?.remaining === "number") {
    return Math.max(0, imageGen.remaining);
  }
  return Math.max(0, account.quota);
}

function isAvailableAccount(account: ImagePolicyAccountLike) {
  return account.status === "正常" && currentImageRemaining(account) > 0;
}

export function sortAccountsForImagePolicy(
  accounts: ImagePolicyAccountLike[],
  sortMode: ImageAccountSortMode,
) {
  return [...accounts].sort((left, right) => {
    if (sortMode === "quota") {
      const quotaDelta = currentImageRemaining(right) - currentImageRemaining(left);
      if (quotaDelta !== 0) {
        return quotaDelta;
      }
    }

    if (sortMode === "imported_at") {
      const importedDelta = parseDateValue(left.importedAt) - parseDateValue(right.importedAt);
      if (importedDelta !== 0) {
        return importedDelta;
      }
    }

    const leftName = `${left.email || ""} ${left.fileName}`.trim().toLowerCase();
    const rightName = `${right.email || ""} ${right.fileName}`.trim().toLowerCase();
    return leftName.localeCompare(rightName);
  });
}

export function buildImageAccountGroupPreviews(
  accounts: ImagePolicyAccountLike[],
  policy: StoredImageAccountPolicy,
) {
  const normalized = normalizeImageAccountPolicy(policy);
  const sortedAccounts = sortAccountsForImagePolicy(accounts, normalized.sortMode);
  const groups: ImageAccountGroupPreview[] = [];

  for (let index = 0; index < sortedAccounts.length; index += normalized.groupSize) {
    const groupAccounts = sortedAccounts.slice(index, index + normalized.groupSize);
    groups.push({
      index: groups.length,
      label: `第 ${groups.length + 1} 组`,
      enabled: normalized.enabledGroupIndexes.includes(groups.length),
      accounts: groupAccounts,
      availableCount: groupAccounts.filter(isAvailableAccount).length,
      totalRemaining: groupAccounts.reduce((sum, account) => sum + currentImageRemaining(account), 0),
      averageRemaining:
        groupAccounts.length > 0
          ? Math.round(
              groupAccounts.reduce((sum, account) => sum + currentImageRemaining(account), 0) / groupAccounts.length,
            )
          : 0,
    });
  }

  return groups;
}
