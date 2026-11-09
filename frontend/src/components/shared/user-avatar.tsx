"use client";

import Image from "next/image";
import { useState } from "react";
import { cn } from "@/lib/utils";

/**
 * Avatar akun/karyawan (EPIC-087): foto profil bila ada, selain itu (atau bila
 * foto gagal dimuat) huruf pertama nama. Warna latar inisial dari className.
 */
export function UserAvatar({
  name,
  src,
  size = 32,
  className,
}: {
  name: string;
  src?: string | null;
  size?: number;
  className?: string;
}) {
  const [broken, setBroken] = useState<string | null>(null);
  const initial = name.trim().slice(0, 1).toUpperCase() || "A";
  const box = cn("grid shrink-0 place-items-center overflow-hidden rounded-full", className);
  if (src && broken !== src) {
    return (
      <span className={box} style={{ width: size, height: size }}>
        <Image
          src={src}
          alt={name}
          width={size}
          height={size}
          unoptimized
          className="size-full object-cover"
          onError={() => setBroken(src)}
        />
      </span>
    );
  }
  return (
    <span className={box} style={{ width: size, height: size }}>
      {initial}
    </span>
  );
}
