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

const UI_COLLAPSED_KEY = "studio.image-account-policy.collapsed.v1";

const defaultPolicy: StoredImageAccountPolicy = {
  enabled: false,
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
  const enabledGroupIndexes = normalizeGroupIndexes(
    value?.enabledGroupIndexes ?? defaultPolicy.enabledGroupIndexes,
  );

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

export function getImageAccountPolicyCollapsed() {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(UI_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export function setImageAccountPolicyCollapsed(value: boolean) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(UI_COLLAPSED_KEY, value ? "1" : "0");
  } catch {
    // ignore localStorage failures
  }
}

export function getEffectiveImageAccountPolicy(
  policy: Partial<StoredImageAccountPolicy> | null | undefined,
  options: {
    groupCount?: number | null;
  } = {},
) {
  const normalized = normalizeImageAccountPolicy(policy);
  const { groupCount } = options;
  if (typeof groupCount !== "number" || groupCount < 0) {
    return normalized;
  }

  return {
    ...normalized,
    enabledGroupIndexes: normalized.enabledGroupIndexes.filter((index) => index < groupCount),
  };
}

function encodeBase64Url(raw: string) {
  const bytes = new TextEncoder().encode(raw);
  let binary = "";
  bytes.forEach((value) => {
    binary += String.fromCharCode(value);
  });
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

export function buildImageAccountPolicyHeader(
  policy = getDefaultImageAccountPolicy(),
) {
  const effectivePolicy = getEffectiveImageAccountPolicy(policy);
  if (!effectivePolicy.enabled || effectivePolicy.enabledGroupIndexes.length === 0) {
    return "";
  }
  return encodeBase64Url(JSON.stringify(effectivePolicy));
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
  const groupCount =
    normalized.groupSize > 0 ? Math.ceil(sortedAccounts.length / normalized.groupSize) : 0;
  const effectivePolicy = getEffectiveImageAccountPolicy(normalized, { groupCount });
  const groups: ImageAccountGroupPreview[] = [];

  for (let index = 0; index < sortedAccounts.length; index += normalized.groupSize) {
    const groupAccounts = sortedAccounts.slice(index, index + normalized.groupSize);
    groups.push({
      index: groups.length,
      label: `第 ${groups.length + 1} 组`,
      enabled: effectivePolicy.enabledGroupIndexes.includes(groups.length),
      accounts: groupAccounts,
      availableCount: groupAccounts.filter(isAvailableAccount).length,
      totalRemaining: groupAccounts.reduce((sum, account) => sum + currentImageRemaining(account), 0),
      averageRemaining:
        groupAccounts.length > 0
          ? Math.round(
              groupAccounts.reduce((sum, account) => sum + currentImageRemaining(account), 0) /
                groupAccounts.length,
            )
          : 0,
    });
  }

  return groups;
}

function isPaidCapableAccount(account: ImagePolicyAccountLike) {
  const type = String((account as { type?: string }).type || "").trim();
  return type === "Plus" || type === "Pro" || type === "Team";
}

export function buildRequestImageAccountPolicy(
  policy: StoredImageAccountPolicy,
  groups: ImageAccountGroupPreview[],
  options: {
    requirePaidAccount?: boolean;
  } = {},
) {
  const normalized = normalizeImageAccountPolicy(policy);
  if (!normalized.enabled) {
    return normalized;
  }

  const availableGroupIndexes = groups.map((group) => group.index);
  let effectiveIndexes = normalized.enabledGroupIndexes.filter((index) =>
    availableGroupIndexes.includes(index),
  );

  if (
    effectiveIndexes.length === 0 &&
    normalized.enabledGroupIndexes.length > 0 &&
    availableGroupIndexes.length > 0
  ) {
    effectiveIndexes = [...availableGroupIndexes];
  }

  if (options.requirePaidAccount) {
    const paidGroupIndexes = groups
      .filter((group) => group.accounts.some(isPaidCapableAccount))
      .map((group) => group.index);
    const selectedPaidIndexes = effectiveIndexes.filter((index) =>
      paidGroupIndexes.includes(index),
    );
    if (selectedPaidIndexes.length > 0) {
      effectiveIndexes = selectedPaidIndexes;
    } else if (paidGroupIndexes.length > 0) {
      effectiveIndexes = [...paidGroupIndexes];
    } else {
      effectiveIndexes = [];
    }
  }

  return {
    ...normalized,
    enabled: normalized.enabled && effectiveIndexes.length > 0,
    enabledGroupIndexes: effectiveIndexes,
  };
}
