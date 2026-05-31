# App-specific R8/ProGuard rules. Most libraries ship consumer rules; add
# keep rules here as release minification surfaces issues (e.g. for
# kotlinx.serialization @Serializable models — handled by the serialization
# plugin's own rules, but verify when the data layer lands).

# SQLCipher: native methods are bound via JNI by name, so the encrypted DB will
# crash at runtime in release builds if R8 strips/renames these classes.
-keep class net.sqlcipher.** { *; }
-keep interface net.sqlcipher.** { *; }
