# kotlinx.serialization: keep generated serializers of the model classes.
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**
-keep,includedescriptorclasses class io.github.mortendnk.portacontrol.**$$serializer { *; }
-keepclassmembers class io.github.mortendnk.portacontrol.** {
    *** Companion;
}
-keepclasseswithmembers class io.github.mortendnk.portacontrol.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# OkHttp / zxing optional dependencies.
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
