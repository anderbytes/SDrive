param([switch]$IncludeInstallers)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$version = (Get-Content -LiteralPath "$projectRoot/frontend/package.json" -Raw | ConvertFrom-Json).version
if (-not ('SDriveResourceCheck' -as [type])) {
    Add-Type @'
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
public static class SDriveResourceCheck {
    delegate bool ResourceCallback(IntPtr module, IntPtr type, IntPtr name, IntPtr parameter);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    static extern IntPtr LoadLibraryEx(string path, IntPtr file, uint flags);
    [DllImport("kernel32.dll")] static extern bool FreeLibrary(IntPtr module);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    static extern bool EnumResourceNames(IntPtr module, IntPtr type, ResourceCallback callback, IntPtr parameter);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    static extern IntPtr FindResource(IntPtr module, IntPtr name, IntPtr type);
    [DllImport("kernel32.dll")] static extern uint SizeofResource(IntPtr module, IntPtr resource);
    [DllImport("kernel32.dll")] static extern IntPtr LoadResource(IntPtr module, IntPtr resource);
    [DllImport("kernel32.dll")] static extern IntPtr LockResource(IntPtr resource);
    static string Digest(byte[] data) {
        using (var sha = SHA256.Create()) return Convert.ToBase64String(sha.ComputeHash(data));
    }
    public static void CheckIcon(string path, string icon, bool application) {
        var expected = new HashSet<string>();
        using (var reader = new BinaryReader(File.OpenRead(icon))) {
            if (reader.ReadUInt16() != 0 || reader.ReadUInt16() != 1 || reader.ReadUInt16() != 7)
                throw new InvalidDataException("Expected a seven-size Windows icon");
            foreach (int size in new[] {16,24,32,48,64,128,256}) {
                int width = reader.ReadByte(), height = reader.ReadByte();
                if (width != size % 256 || height != size % 256)
                    throw new InvalidDataException("Unexpected icon dimensions");
                reader.ReadUInt16(); reader.ReadUInt16(); reader.ReadUInt16();
                int length = reader.ReadInt32(), offset = reader.ReadInt32();
                long next = reader.BaseStream.Position;
                reader.BaseStream.Position = offset;
                byte[] frame = reader.ReadBytes(length);
                if (frame.Length != length) throw new InvalidDataException("Truncated icon frame");
                expected.Add(Digest(frame));
                reader.BaseStream.Position = next;
            }
        }
        IntPtr module = LoadLibraryEx(path, IntPtr.Zero, 0x22);
        if (module == IntPtr.Zero) throw new Win32Exception(Marshal.GetLastWin32Error());
        try {
            if (application && FindResource(module, (IntPtr)3, (IntPtr)14) == IntPtr.Zero)
                throw new InvalidDataException("Missing Wails window icon resource 3");
            var actual = new HashSet<string>();
            ResourceCallback callback = (m, type, name, parameter) => {
                IntPtr resource = FindResource(m, name, type);
                byte[] data = new byte[checked((int)SizeofResource(m, resource))];
                Marshal.Copy(LockResource(LoadResource(m, resource)), data, 0, data.Length);
                actual.Add(Digest(data));
                return true;
            };
            if (!EnumResourceNames(module, (IntPtr)3, callback, IntPtr.Zero))
                throw new Win32Exception(Marshal.GetLastWin32Error());
            if (!expected.IsSubsetOf(actual)) throw new InvalidDataException("Executable icon differs from source: " + path);
        } finally { FreeLibrary(module); }
    }
}
'@
}
$artifacts = @('SDrive.exe', 'SDrive-check.exe')
if ($IncludeInstallers) { $artifacts += @("SDrive-$version-setup.exe", "SDrive-$version-offline-setup.exe") }
foreach ($artifact in $artifacts) {
    $path = Join-Path $projectRoot "bin/$artifact"
    $metadata = [Diagnostics.FileVersionInfo]::GetVersionInfo($path)
    if ($metadata.FileVersion -cne $version -or $metadata.ProductVersion -cne $version -or $metadata.ProductName -cne 'SDrive') {
        throw "Incorrect Windows version metadata: $artifact"
    }
    [SDriveResourceCheck]::CheckIcon($path, "$projectRoot/assets/sdrive.ico", $artifact -in @('SDrive.exe', 'SDrive-check.exe'))
    Write-Output "Verified version $version and all seven icon sizes: $artifact"
}
