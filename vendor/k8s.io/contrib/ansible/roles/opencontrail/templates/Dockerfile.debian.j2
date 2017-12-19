FROM {{ ansible_distribution | lower }}:{{ ansible_distribution_version }}
RUN apt-get update
RUN {{ ansible_pkg_mgr }} install -y git make
RUN {{ ansible_pkg_mgr }} install -y automake flex bison gcc g++ scons linux-headers-{{ ansible_kernel }} libboost-dev libxml2-dev
RUN mkdir -p src/vrouter
WORKDIR src/vrouter
RUN git clone -b R2.20 https://github.com/Juniper/contrail-vrouter vrouter
RUN mkdir tools
RUN (cd tools && git clone https://github.com/Juniper/contrail-build build)
RUN (cd tools && git clone -b R2.20 https://github.com/Juniper/contrail-sandesh sandesh)
RUN cp tools/build/SConstruct .

