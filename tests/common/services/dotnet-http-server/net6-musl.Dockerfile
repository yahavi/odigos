FROM odigosdemo.jfrog.io/docker/dotnet/sdk:6.0-alpine AS build
WORKDIR /src
COPY . .
ENV USE_DOTNET6=true
RUN dotnet restore
RUN dotnet publish -c Release -o /app

FROM odigosdemo.jfrog.io/docker/dotnet/aspnet:6.0-alpine
WORKDIR /app
COPY --from=build /app .
EXPOSE 8080
ENTRYPOINT ["dotnet", "dotnet-http-server.dll"]
