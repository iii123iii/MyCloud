"use client";

import { Fragment } from "react";
import {
  Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList,
  BreadcrumbPage, BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Home } from "lucide-react";
import type { FolderItem } from "@/lib/types";

interface Props {
  path: FolderItem[];
  onNavigate: (index: number) => void;
}

export function BreadcrumbNav({ path, onNavigate }: Props) {
  return (
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem>
          {path.length === 0 ? (
            <BreadcrumbPage className="flex items-center gap-1">
              <Home className="h-3.5 w-3.5" />
              My Files
            </BreadcrumbPage>
          ) : (
            <BreadcrumbLink
              className="flex items-center gap-1 cursor-pointer"
              onClick={() => onNavigate(-1)}
            >
              <Home className="h-3.5 w-3.5" />
              My Files
            </BreadcrumbLink>
          )}
        </BreadcrumbItem>

        {path.map((folder, i) => (
          <Fragment key={folder.id}>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              {i === path.length - 1 ? (
                <BreadcrumbPage>{folder.name}</BreadcrumbPage>
              ) : (
                <BreadcrumbLink
                  className="cursor-pointer"
                  onClick={() => onNavigate(i)}
                >
                  {folder.name}
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          </Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
